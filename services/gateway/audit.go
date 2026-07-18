package gateway

import (
	"log/slog"
	"time"
)

type AuditEvent struct {
	RequestID string
	ClientID  string
	Route     string
	Model     string
	WorkerID  string
	Stream    bool
	Status    int
	LatencyMs int64
	PromptTok int32
	CompTok   int32
	Error     string
}

func LogAudit(logger *slog.Logger, e AuditEvent) {
	if logger == nil {
		return
	}
	attrs := []any{
		"request_id", e.RequestID,
		"client_id", e.ClientID,
		"route", e.Route,
		"model", e.Model,
		"worker_id", e.WorkerID,
		"stream", e.Stream,
		"status", e.Status,
		"latency_ms", e.LatencyMs,
		"prompt_tokens", e.PromptTok,
		"completion_tokens", e.CompTok,
	}
	if e.Error != "" {
		attrs = append(attrs, "error", e.Error)
		logger.Warn("gateway_audit", attrs...)
		return
	}
	logger.Info("gateway_audit", attrs...)
}

func auditLatency(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}
