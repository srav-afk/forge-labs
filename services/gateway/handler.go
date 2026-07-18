package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	workerv1 "github.com/srav-afk/forge-labs/gen/worker/v1"
	"github.com/srav-afk/forge-labs/services/routing"
	"github.com/srav-afk/forge-labs/services/scheduler"
)

type Handler struct {
	selector       WorkerSelector
	inflight       *routing.InflightTracker
	latency        *scheduler.LatencyStore
	metrics        *Metrics
	admissionLimit int64
	retryAfterSec  int
	dial           func(ctx context.Context, endpoint string) (workerv1.WorkerServiceClient, func(), error)
}

func NewHandler(
	selector WorkerSelector,
	inflight *routing.InflightTracker,
	latency *scheduler.LatencyStore,
	metrics *Metrics,
	admissionLimit int,
	retryAfterSec int,
) *Handler {
	if admissionLimit <= 0 {
		admissionLimit = 4
	}
	if retryAfterSec <= 0 {
		retryAfterSec = 2
	}
	return &Handler{
		selector:       selector,
		inflight:       inflight,
		latency:        latency,
		metrics:        metrics,
		admissionLimit: int64(admissionLimit),
		retryAfterSec:  retryAfterSec,
		dial:           dialWorker,
	}
}

func (h *Handler) observeLatency(workerID string, started time.Time) {
	if h.latency == nil || workerID == "" {
		return
	}
	h.latency.Observe(workerID, float64(time.Since(started).Milliseconds()))
}

func (h *Handler) tryAdmit(workerID string) (func(), bool) {
	if h.inflight == nil {
		return func() {}, true
	}
	release, ok := h.inflight.TryTrack(workerID, h.admissionLimit)
	if !ok {
		return nil, false
	}
	if h.metrics != nil {
		h.metrics.SetInflight(workerID, h.inflight.Get(workerID))
	}
	return func() {
		release()
		if h.metrics != nil {
			h.metrics.SetInflight(workerID, h.inflight.Get(workerID))
		}
	}, true
}

func (h *Handler) Register(r *gin.Engine) {
	r.POST("/v1/chat/completions", h.chatCompletions)
	r.POST("/v1/completions", h.completions)
	r.GET("/v1/models", h.listModels)
}

func dialWorker(ctx context.Context, endpoint string) (workerv1.WorkerServiceClient, func(), error) {
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}
	return workerv1.NewWorkerServiceClient(conn), func() { _ = conn.Close() }, nil
}

func (h *Handler) listModels(c *gin.Context) {
	c.JSON(http.StatusOK, modelsResponse{
		Object: "list",
		Data:   h.selector.ListModels(),
	})
}

func (h *Handler) chatCompletions(c *gin.Context) {
	start := time.Now()
	var req chatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_request")
		return
	}
	if req.Model == "" {
		writeOpenAIError(c, http.StatusBadRequest, "model is required", "invalid_request_error", "invalid_request")
		return
	}
	if len(req.Messages) == 0 {
		writeOpenAIError(c, http.StatusBadRequest, "messages is required", "invalid_request_error", "invalid_request")
		return
	}

	statusCode := http.StatusOK
	defer func() {
		h.metrics.ObserveDuration("chat_completions", req.Model, req.Stream, statusCode, time.Since(start).Seconds())
	}()

	worker, err := h.selector.SelectWorker(req.Model)
	if err != nil {
		statusCode, typ, code := selectErrorStatus(err)
		if errors.Is(err, scheduler.ErrAdmissionRejected) {
			h.metrics.IncRejected(req.Model, "fleet_saturated")
			writeAdmissionRejected(c, h.retryAfterSec)
			return
		}
		if statusCode == http.StatusServiceUnavailable || statusCode == http.StatusNotFound {
			h.metrics.IncRejected(req.Model, code)
		}
		writeOpenAIError(c, statusCode, err.Error(), typ, code)
		return
	}
	done, ok := h.tryAdmit(worker.ID)
	if !ok {
		statusCode = http.StatusTooManyRequests
		h.metrics.IncRejected(req.Model, "fleet_saturated")
		writeAdmissionRejected(c, h.retryAfterSec)
		return
	}
	defer done()
	defer h.observeLatency(worker.ID, start)
	h.metrics.IncAdmitted(req.Model)

	prompt := messagesToPrompt(req.Messages)
	genReq := toWorkerRequest(req.Model, prompt, req.Temperature, req.TopP, maxTokensFromChat(req))
	includeUsage := req.StreamOptions != nil && req.StreamOptions.IncludeUsage

	if req.Stream {
		if err := h.streamChat(c, worker, genReq, req.Model, includeUsage, start); err != nil {
			if !c.Writer.Written() {
				httpStatus, msg, typ, code := mapGRPCError(err)
				statusCode = httpStatus
				writeOpenAIError(c, httpStatus, msg, typ, code)
			}
		}
		return
	}

	text, usage, finish, err := h.collect(c.Request.Context(), worker, genReq)
	if err != nil {
		httpStatus, msg, typ, code := mapGRPCError(err)
		statusCode = httpStatus
		writeOpenAIError(c, httpStatus, msg, typ, code)
		return
	}
	if finish == "" {
		finish = "stop"
	}
	c.JSON(http.StatusOK, chatCompletionResponse{
		ID:      newCompletionID("chatcmpl"),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []chatResponseChoice{{
			Index:        0,
			Message:      chatMessage{Role: "assistant", Content: text},
			FinishReason: finish,
		}},
		Usage: usageOrZero(usage),
	})
}

func (h *Handler) completions(c *gin.Context) {
	start := time.Now()
	var req completionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeOpenAIError(c, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_request")
		return
	}
	if req.Model == "" {
		writeOpenAIError(c, http.StatusBadRequest, "model is required", "invalid_request_error", "invalid_request")
		return
	}
	prompt, err := promptFromCompletions(req.Prompt)
	if err != nil || prompt == "" {
		writeOpenAIError(c, http.StatusBadRequest, "prompt is required", "invalid_request_error", "invalid_request")
		return
	}

	statusCode := http.StatusOK
	defer func() {
		h.metrics.ObserveDuration("completions", req.Model, req.Stream, statusCode, time.Since(start).Seconds())
	}()

	worker, err := h.selector.SelectWorker(req.Model)
	if err != nil {
		statusCode, typ, code := selectErrorStatus(err)
		if errors.Is(err, scheduler.ErrAdmissionRejected) {
			h.metrics.IncRejected(req.Model, "fleet_saturated")
			writeAdmissionRejected(c, h.retryAfterSec)
			return
		}
		if statusCode == http.StatusServiceUnavailable || statusCode == http.StatusNotFound {
			h.metrics.IncRejected(req.Model, code)
		}
		writeOpenAIError(c, statusCode, err.Error(), typ, code)
		return
	}
	done, ok := h.tryAdmit(worker.ID)
	if !ok {
		statusCode = http.StatusTooManyRequests
		h.metrics.IncRejected(req.Model, "fleet_saturated")
		writeAdmissionRejected(c, h.retryAfterSec)
		return
	}
	defer done()
	defer h.observeLatency(worker.ID, start)
	h.metrics.IncAdmitted(req.Model)

	genReq := toWorkerRequest(req.Model, prompt, req.Temperature, req.TopP, req.MaxTokens)
	includeUsage := req.StreamOptions != nil && req.StreamOptions.IncludeUsage

	if req.Stream {
		if err := h.streamText(c, worker, genReq, req.Model, includeUsage, start); err != nil {
			if !c.Writer.Written() {
				httpStatus, msg, typ, code := mapGRPCError(err)
				statusCode = httpStatus
				writeOpenAIError(c, httpStatus, msg, typ, code)
			}
		}
		return
	}

	text, usage, finish, err := h.collect(c.Request.Context(), worker, genReq)
	if err != nil {
		httpStatus, msg, typ, code := mapGRPCError(err)
		statusCode = httpStatus
		writeOpenAIError(c, httpStatus, msg, typ, code)
		return
	}
	if finish == "" {
		finish = "stop"
	}
	fr := finish
	c.JSON(http.StatusOK, textCompletionResponse{
		ID:      newCompletionID("cmpl"),
		Object:  "text_completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []textCompletionChoice{{
			Index:        0,
			Text:         text,
			FinishReason: &fr,
		}},
		Usage: usageOrZero(usage),
	})
}

func (h *Handler) streamChat(c *gin.Context, worker *SelectedWorker, genReq *workerv1.GenerateRequest, model string, includeUsage bool, start time.Time) error {
	client, closer, err := h.dial(c.Request.Context(), worker.Endpoint)
	if err != nil {
		return status.Errorf(codes.Unavailable, "dial worker: %v", err)
	}
	defer closer()

	stream, err := client.Generate(c.Request.Context(), genReq)
	if err != nil {
		return err
	}

	setSSEHeaders(c.Writer)
	c.Status(http.StatusOK)
	flusher, _ := c.Writer.(http.Flusher)

	id := newCompletionID("chatcmpl")
	created := time.Now().Unix()
	first := true
	var finalUsage *usage
	var finishReason string

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if first {
				return err
			}
			_ = writeSSEError(c.Writer, flusher, err.Error())
			return nil
		}

		if first {
			first = false
			h.metrics.ObserveTTFT("chat_completions", model, time.Since(start).Seconds())
			roleChunk := chatCompletionChunk{
				ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
				Choices: []chatChunkChoice{{Index: 0, Delta: chatDelta{Role: "assistant"}}},
			}
			if err := writeSSEData(c.Writer, flusher, roleChunk); err != nil {
				return err
			}
		}

		if chunk.GetText() != "" {
			msg := chatCompletionChunk{
				ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
				Choices: []chatChunkChoice{{Index: 0, Delta: chatDelta{Content: chunk.GetText()}}},
			}
			if err := writeSSEData(c.Writer, flusher, msg); err != nil {
				return err
			}
		}

		if chunk.GetDone() {
			finishReason = chunk.GetFinishReason()
			if finishReason == "" {
				finishReason = "stop"
			}
			if u := chunk.GetUsage(); u != nil {
				finalUsage = &usage{
					PromptTokens:     u.GetPromptTokens(),
					CompletionTokens: u.GetCompletionTokens(),
					TotalTokens:      u.GetPromptTokens() + u.GetCompletionTokens(),
				}
			}
		}
	}

	fr := finishReason
	if fr == "" {
		fr = "stop"
	}
	stopChunk := chatCompletionChunk{
		ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
		Choices: []chatChunkChoice{{Index: 0, Delta: chatDelta{}, FinishReason: &fr}},
	}
	if err := writeSSEData(c.Writer, flusher, stopChunk); err != nil {
		return err
	}

	if includeUsage && finalUsage != nil {
		usageChunk := chatCompletionChunk{
			ID: id, Object: "chat.completion.chunk", Created: created, Model: model,
			Choices: []chatChunkChoice{},
			Usage:   finalUsage,
		}
		if err := writeSSEData(c.Writer, flusher, usageChunk); err != nil {
			return err
		}
	}
	return writeSSEDone(c.Writer, flusher)
}

func (h *Handler) streamText(c *gin.Context, worker *SelectedWorker, genReq *workerv1.GenerateRequest, model string, includeUsage bool, start time.Time) error {
	client, closer, err := h.dial(c.Request.Context(), worker.Endpoint)
	if err != nil {
		return status.Errorf(codes.Unavailable, "dial worker: %v", err)
	}
	defer closer()

	stream, err := client.Generate(c.Request.Context(), genReq)
	if err != nil {
		return err
	}

	setSSEHeaders(c.Writer)
	c.Status(http.StatusOK)
	flusher, _ := c.Writer.(http.Flusher)

	id := newCompletionID("cmpl")
	created := time.Now().Unix()
	first := true
	var finalUsage *usage
	var finishReason string

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if first {
				return err
			}
			_ = writeSSEError(c.Writer, flusher, err.Error())
			return nil
		}
		if first {
			first = false
			h.metrics.ObserveTTFT("completions", model, time.Since(start).Seconds())
		}
		if chunk.GetText() != "" {
			msg := textCompletionChunk{
				ID: id, Object: "text_completion", Created: created, Model: model,
				Choices: []textCompletionChoice{{Index: 0, Text: chunk.GetText()}},
			}
			if err := writeSSEData(c.Writer, flusher, msg); err != nil {
				return err
			}
		}
		if chunk.GetDone() {
			finishReason = chunk.GetFinishReason()
			if finishReason == "" {
				finishReason = "stop"
			}
			if u := chunk.GetUsage(); u != nil {
				finalUsage = &usage{
					PromptTokens:     u.GetPromptTokens(),
					CompletionTokens: u.GetCompletionTokens(),
					TotalTokens:      u.GetPromptTokens() + u.GetCompletionTokens(),
				}
			}
		}
	}

	fr := finishReason
	if fr == "" {
		fr = "stop"
	}
	stopChunk := textCompletionChunk{
		ID: id, Object: "text_completion", Created: created, Model: model,
		Choices: []textCompletionChoice{{Index: 0, Text: "", FinishReason: &fr}},
	}
	if err := writeSSEData(c.Writer, flusher, stopChunk); err != nil {
		return err
	}
	if includeUsage && finalUsage != nil {
		usageChunk := textCompletionChunk{
			ID: id, Object: "text_completion", Created: created, Model: model,
			Choices: []textCompletionChoice{},
			Usage:   finalUsage,
		}
		if err := writeSSEData(c.Writer, flusher, usageChunk); err != nil {
			return err
		}
	}
	return writeSSEDone(c.Writer, flusher)
}

func (h *Handler) collect(ctx context.Context, worker *SelectedWorker, genReq *workerv1.GenerateRequest) (string, *usage, string, error) {
	client, closer, err := h.dial(ctx, worker.Endpoint)
	if err != nil {
		return "", nil, "", status.Errorf(codes.Unavailable, "dial worker: %v", err)
	}
	defer closer()

	stream, err := client.Generate(ctx, genReq)
	if err != nil {
		return "", nil, "", err
	}

	var b strings.Builder
	var u *usage
	var finish string
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", nil, "", err
		}
		b.WriteString(chunk.GetText())
		if chunk.GetDone() {
			finish = chunk.GetFinishReason()
			if usageMsg := chunk.GetUsage(); usageMsg != nil {
				u = &usage{
					PromptTokens:     usageMsg.GetPromptTokens(),
					CompletionTokens: usageMsg.GetCompletionTokens(),
					TotalTokens:      usageMsg.GetPromptTokens() + usageMsg.GetCompletionTokens(),
				}
			}
		}
	}
	return b.String(), u, finish, nil
}

func usageOrZero(u *usage) usage {
	if u == nil {
		return usage{}
	}
	return *u
}

func newCompletionID(prefix string) string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b[:]))
}
