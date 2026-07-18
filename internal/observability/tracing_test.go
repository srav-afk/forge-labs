package observability

import (
	"context"
	"testing"
)

func TestNewTracerNoopWithoutEndpoint(t *testing.T) {
	tr, err := NewTracer(TraceConfig{ServiceName: "test", OTLPEndpoint: ""})
	if err != nil {
		t.Fatal(err)
	}
	ctx, span := tr.Tracer().Start(context.Background(), "unit")
	span.End()
	if err := tr.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}
