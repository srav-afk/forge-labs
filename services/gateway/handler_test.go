package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	workerv1 "github.com/srav-afk/forge-labs/gen/worker/v1"
	"github.com/srav-afk/forge-labs/internal/observability"
	"github.com/srav-afk/forge-labs/services/routing"
	"github.com/srav-afk/forge-labs/services/scheduler"
)

type staticSelector struct {
	w *SelectedWorker
}

func (s staticSelector) SelectWorker(string) (*SelectedWorker, error) { return s.w, nil }
func (s staticSelector) ListModels() []modelObject {
	return []modelObject{{ID: "llama3.2", Object: "model", Created: 1, OwnedBy: "forge"}}
}

type fakeWorker struct {
	workerv1.UnimplementedWorkerServiceServer
	chunks []*workerv1.TokenChunk
	err    error
}

func (f *fakeWorker) Generate(req *workerv1.GenerateRequest, stream workerv1.WorkerService_GenerateServer) error {
	if f.err != nil {
		return f.err
	}
	for _, c := range f.chunks {
		if err := stream.Send(c); err != nil {
			return err
		}
	}
	return nil
}

func testHandler(t *testing.T, fw *fakeWorker) (*Handler, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer()
	workerv1.RegisterWorkerServiceServer(s, fw)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)

	reg := observability.NewRegistry()
	m := NewMetrics(reg)
	inf := routing.NewInflightTracker()
	lat := scheduler.NewLatencyStore(10*time.Second, nil)
	h := NewHandler(staticSelector{w: &SelectedWorker{ID: "w1", Endpoint: "bufnet", Models: []string{"llama3.2"}}}, inf, lat, m)
	h.dial = func(ctx context.Context, endpoint string) (workerv1.WorkerServiceClient, func(), error) {
		conn, err := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return nil, nil, err
		}
		return workerv1.NewWorkerServiceClient(conn), func() { _ = conn.Close() }, nil
	}

	r := gin.New()
	h.Register(r)
	return h, httptest.NewServer(r)
}

func TestChatCompletionsStream(t *testing.T) {
	fw := &fakeWorker{chunks: []*workerv1.TokenChunk{
		{Text: "Hello", Done: false},
		{Text: " there", Done: false},
		{Text: "", Done: true, FinishReason: "stop", Usage: &workerv1.Usage{PromptTokens: 17, CompletionTokens: 3}},
	}}
	_, srv := testHandler(t, fw)
	defer srv.Close()

	body := `{"model":"llama3.2","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"Say hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, b)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type=%s", ct)
	}

	sc := bufio.NewScanner(resp.Body)
	var events []string
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data: ") {
			events = append(events, strings.TrimPrefix(line, "data: "))
		}
	}
	if len(events) < 4 {
		t.Fatalf("events=%v", events)
	}
	if events[len(events)-1] != "[DONE]" {
		t.Fatalf("last=%q", events[len(events)-1])
	}
	// usage chunk should be second-to-last when include_usage
	var usageChunk chatCompletionChunk
	if err := json.Unmarshal([]byte(events[len(events)-2]), &usageChunk); err != nil {
		t.Fatal(err)
	}
	if usageChunk.Usage == nil || usageChunk.Usage.PromptTokens != 17 {
		t.Fatalf("usage chunk=%+v", usageChunk)
	}
	if len(usageChunk.Choices) != 0 {
		t.Fatalf("usage choices should be empty: %+v", usageChunk.Choices)
	}
}

func TestChatCompletionsModelNotFound(t *testing.T) {
	fw := &fakeWorker{err: status.Error(codes.NotFound, `model "missing" not found`)}
	_, srv := testHandler(t, fw)
	defer srv.Close()

	body := `{"model":"missing","stream":false,"messages":[{"role":"user","content":"hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var errBody openAIErrorBody
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
		t.Fatal(err)
	}
	if errBody.Error.Code != "model_not_found" {
		t.Fatalf("code=%s", errBody.Error.Code)
	}
}

func TestListModels(t *testing.T) {
	_, srv := testHandler(t, &fakeWorker{})
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Object != "list" || len(out.Data) != 1 || out.Data[0].OwnedBy != "forge" {
		t.Fatalf("%+v", out)
	}
}

func TestStreamCancelPropagates(t *testing.T) {
	started := make(chan struct{})
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer()
	workerv1.RegisterWorkerServiceServer(s, &blockingWorker{started: started})
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)

	gin.SetMode(gin.TestMode)
	reg := observability.NewRegistry()
	h := NewHandler(staticSelector{w: &SelectedWorker{ID: "w1", Endpoint: "bufnet"}}, routing.NewInflightTracker(), scheduler.NewLatencyStore(10*time.Second, nil), NewMetrics(reg))
	h.dial = func(ctx context.Context, endpoint string) (workerv1.WorkerServiceClient, func(), error) {
		conn, err := grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return nil, nil, err
		}
		return workerv1.NewWorkerServiceClient(conn), func() { _ = conn.Close() }, nil
	}
	r := gin.New()
	h.Register(r)
	srv := httptest.NewServer(r)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/v1/chat/completions",
		bytes.NewReader([]byte(`{"model":"m","stream":true,"messages":[{"role":"user","content":"x"}]}`)))
	req.Header.Set("Content-Type", "application/json")
	go func() {
		<-started
		cancel()
	}()
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	time.Sleep(50 * time.Millisecond)
}

type blockingWorker struct {
	workerv1.UnimplementedWorkerServiceServer
	started chan struct{}
}

func (b *blockingWorker) Generate(req *workerv1.GenerateRequest, stream workerv1.WorkerService_GenerateServer) error {
	close(b.started)
	<-stream.Context().Done()
	return stream.Context().Err()
}
