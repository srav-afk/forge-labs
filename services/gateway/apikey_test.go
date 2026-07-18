package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestKeyStoreEnvLookup(t *testing.T) {
	s := NewKeyStore(nil)
	s.LoadFromEnv("sk-alice:alice:2,sk-bob:bob", "legacy")
	if !s.Required() {
		t.Fatal("expected required")
	}
	id, ok := s.Lookup("sk-alice")
	if !ok || id.ClientID != "alice" || id.MaxConcurrent != 2 {
		t.Fatalf("%+v %v", id, ok)
	}
	if _, ok := s.Lookup("nope"); ok {
		t.Fatal("expected miss")
	}
	if id, ok := s.Lookup("legacy"); !ok || id.ClientID != "default" {
		t.Fatalf("legacy %+v", id)
	}
}

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keys := NewKeyStore(nil)
	keys.LoadFromEnv("good:testclient:1", "")
	lim := GatewayLimits{
		Keys:           keys,
		Limiter:        NewConcurrencyLimiter(),
		RequestTimeout: time.Second,
		MaxBodyBytes:   1024,
	}
	r := gin.New()
	r.Use(lim.Middleware())
	r.GET("/v1/models", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer good")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200 got %d body=%s", w.Code, w.Body.String())
	}
	if w.Header().Get(headerRequestID) == "" {
		t.Fatal("missing request id")
	}
}
