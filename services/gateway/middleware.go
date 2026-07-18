package gateway

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	ctxKeyClient   = "forge_client"
	ctxKeyRequest  = "forge_request_id"
	headerRequestID = "X-Request-Id"
)

type ConcurrencyLimiter struct {
	mu   sync.Mutex
	by   map[string]*int64
	maxDefault int
}

func NewConcurrencyLimiter() *ConcurrencyLimiter {
	return &ConcurrencyLimiter{by: map[string]*int64{}, maxDefault: 32}
}

func (l *ConcurrencyLimiter) Acquire(clientID string, max int) (func(), bool) {
	if max <= 0 {
		max = l.maxDefault
	}
	l.mu.Lock()
	ctr, ok := l.by[clientID]
	if !ok {
		var z int64
		ctr = &z
		l.by[clientID] = ctr
	}
	l.mu.Unlock()
	for {
		cur := atomic.LoadInt64(ctr)
		if cur >= int64(max) {
			return nil, false
		}
		if atomic.CompareAndSwapInt64(ctr, cur, cur+1) {
			return func() { atomic.AddInt64(ctr, -1) }, true
		}
	}
}

type GatewayLimits struct {
	RequestTimeout time.Duration
	MaxBodyBytes   int64
	Keys           *KeyStore
	Limiter        *ConcurrencyLimiter
	Logger         *slog.Logger
}

func (lim GatewayLimits) Middleware() gin.HandlerFunc {
	if lim.Limiter == nil {
		lim.Limiter = NewConcurrencyLimiter()
	}
	if lim.RequestTimeout <= 0 {
		lim.RequestTimeout = 5 * time.Minute
	}
	if lim.MaxBodyBytes <= 0 {
		lim.MaxBodyBytes = 1 << 20
	}
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/healthz" || path == "/internal/cache/events" {
			c.Next()
			return
		}

		reqID := c.GetHeader(headerRequestID)
		if reqID == "" {
			reqID = newCompletionID("req")
		}
		c.Set(ctxKeyRequest, reqID)
		c.Writer.Header().Set(headerRequestID, reqID)

		if lim.Keys != nil && lim.Keys.Required() {
			raw := extractAPIKey(c)
			id, ok := lim.Keys.Lookup(raw)
			if !ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": gin.H{"message": "invalid api key", "type": "auth_error", "code": "unauthorized"},
				})
				return
			}
			c.Set(ctxKeyClient, id)
			release, ok := lim.Limiter.Acquire(id.ClientID, id.MaxConcurrent)
			if !ok {
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"error": gin.H{
						"message": "client concurrency limit",
						"type":    "rate_limit_error",
						"code":    "client_saturated",
					},
				})
				return
			}
			defer release()
		}

		if c.Request.Body != nil && lim.MaxBodyBytes > 0 {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, lim.MaxBodyBytes)
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), lim.RequestTimeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)

		start := time.Now()
		c.Next()

		clientID := "anonymous"
		if v, ok := c.Get(ctxKeyClient); ok {
			if id, ok := v.(ClientIdentity); ok {
				clientID = id.ClientID
			}
		}
		if lim.Logger != nil {
			lim.Logger.Info("gateway_request",
				"request_id", reqID,
				"client_id", clientID,
				"method", c.Request.Method,
				"path", path,
				"status", c.Writer.Status(),
				"latency_ms", time.Since(start).Milliseconds(),
				"bytes_out", c.Writer.Size(),
			)
		}
	}
}

func extractAPIKey(c *gin.Context) string {
	auth := c.GetHeader("Authorization")
	if len(auth) > 7 && (auth[:7] == "Bearer " || auth[:7] == "bearer ") {
		return auth[7:]
	}
	if k := c.GetHeader("X-API-Key"); k != "" {
		return k
	}
	return ""
}

func ClientFromContext(c *gin.Context) ClientIdentity {
	if v, ok := c.Get(ctxKeyClient); ok {
		if id, ok := v.(ClientIdentity); ok {
			return id
		}
	}
	return ClientIdentity{ClientID: "anonymous", MaxConcurrent: 32}
}

func RequestIDFromContext(c *gin.Context) string {
	if v, ok := c.Get(ctxKeyRequest); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
