package reliability

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFailoverSkipsToNextOnUnavailable(t *testing.T) {
	var tried []string
	fo := NewFailover(NewRetryBudget(100, 0.1), NewBreakerMap(DefaultBreakerConfig(), nil), nil, 3)
	workers := []Worker{{ID: "a", Endpoint: "a"}, {ID: "b", Endpoint: "b"}}
	used, err := fo.Do(context.Background(), workers, func(ctx context.Context, w Worker) error {
		tried = append(tried, w.ID)
		if w.ID == "a" {
			return status.Error(codes.Unavailable, "down")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if used.ID != "b" {
		t.Fatalf("used=%s", used.ID)
	}
	if len(tried) != 2 || tried[0] != "a" {
		t.Fatalf("tried=%v", tried)
	}
}

func TestRetryBudgetSuppresses(t *testing.T) {
	b := NewRetryBudget(10, 0.1)
	for i := 0; i < 6; i++ {
		b.OnFailure()
	}
	if b.Allow() {
		t.Fatalf("tokens=%v should suppress", b.Tokens())
	}
	for i := 0; i < 50; i++ {
		b.OnSuccess()
	}
	if !b.Allow() {
		t.Fatalf("tokens=%v should recover", b.Tokens())
	}
}

func TestBreakerOpens(t *testing.T) {
	m := NewBreakerMap(BreakerConfig{
		MinRequests:  5,
		FailureRatio: 0.5,
		Timeout:      5 * time.Second,
		MaxHalfOpen:  1,
	}, nil)
	for i := 0; i < 5; i++ {
		_ = m.Allow("w1")
		m.Failure("w1")
	}
	if m.State("w1") != StateOpen {
		t.Fatalf("state=%v", m.State("w1"))
	}
	if err := m.Allow("w1"); !errors.Is(err, ErrOpen) {
		t.Fatalf("err=%v", err)
	}
}

func TestNonRetryableStops(t *testing.T) {
	fo := NewFailover(NewRetryBudget(100, 0.1), nil, nil, 3)
	workers := []Worker{{ID: "a"}, {ID: "b"}}
	_, err := fo.Do(context.Background(), workers, func(ctx context.Context, w Worker) error {
		return status.Error(codes.InvalidArgument, "bad")
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("err=%v", err)
	}
}
