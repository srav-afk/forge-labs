package reliability

import (
	"errors"
	"sync"
	"time"
)

var ErrOpen = errors.New("circuit breaker open")

type State int

const (
	StateClosed State = iota
	StateHalfOpen
	StateOpen
)

type BreakerConfig struct {
	MinRequests  uint32
	FailureRatio float64
	Timeout      time.Duration
	MaxHalfOpen  uint32
}

func DefaultBreakerConfig() BreakerConfig {
	return BreakerConfig{
		MinRequests:  10,
		FailureRatio: 0.5,
		Timeout:      5 * time.Second,
		MaxHalfOpen:  2,
	}
}

type BreakerMap struct {
	mu      sync.Mutex
	m       map[string]*workerBreaker
	cfg     BreakerConfig
	onState func(workerID string, state State)
}

type workerBreaker struct {
	state        State
	requests     uint32
	failures     uint32
	successes    uint32
	halfOpenLeft uint32
	openedAt     time.Time
}

func NewBreakerMap(cfg BreakerConfig, onState func(string, State)) *BreakerMap {
	if cfg.MinRequests == 0 {
		cfg.MinRequests = 10
	}
	if cfg.FailureRatio <= 0 {
		cfg.FailureRatio = 0.5
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.MaxHalfOpen == 0 {
		cfg.MaxHalfOpen = 2
	}
	return &BreakerMap{m: make(map[string]*workerBreaker), cfg: cfg, onState: onState}
}

func (b *BreakerMap) Allow(workerID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	wb := b.get(workerID)
	now := time.Now()
	switch wb.state {
	case StateOpen:
		if now.Sub(wb.openedAt) >= b.cfg.Timeout {
			b.setState(workerID, wb, StateHalfOpen)
			wb.halfOpenLeft = b.cfg.MaxHalfOpen
		} else {
			return ErrOpen
		}
	case StateHalfOpen:
		if wb.halfOpenLeft == 0 {
			return ErrOpen
		}
		wb.halfOpenLeft--
	}
	return nil
}

func (b *BreakerMap) Success(workerID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	wb := b.get(workerID)
	wb.requests++
	wb.successes++
	if wb.state == StateHalfOpen {
		b.setState(workerID, wb, StateClosed)
		wb.requests, wb.failures, wb.successes = 0, 0, 0
		return
	}
	if wb.state == StateClosed && wb.requests >= b.cfg.MinRequests*2 {
		// decay window
		wb.requests /= 2
		wb.failures /= 2
		wb.successes /= 2
	}
}

func (b *BreakerMap) Failure(workerID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	wb := b.get(workerID)
	wb.requests++
	wb.failures++
	if wb.state == StateHalfOpen {
		b.setState(workerID, wb, StateOpen)
		wb.openedAt = time.Now()
		return
	}
	if wb.requests >= b.cfg.MinRequests {
		ratio := float64(wb.failures) / float64(wb.requests)
		if ratio >= b.cfg.FailureRatio {
			b.setState(workerID, wb, StateOpen)
			wb.openedAt = time.Now()
		}
	}
}

func (b *BreakerMap) State(workerID string) State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.get(workerID).state
}

func (b *BreakerMap) get(workerID string) *workerBreaker {
	wb, ok := b.m[workerID]
	if !ok {
		wb = &workerBreaker{state: StateClosed}
		b.m[workerID] = wb
	}
	return wb
}

func (b *BreakerMap) setState(workerID string, wb *workerBreaker, to State) {
	from := wb.state
	if from == to {
		return
	}
	wb.state = to
	if b.onState != nil {
		b.onState(workerID, to)
	}
}
