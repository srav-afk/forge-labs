package reliability

import "sync"

type RetryBudget struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	tokenRatio float64
}

func NewRetryBudget(maxTokens, tokenRatio float64) *RetryBudget {
	if maxTokens <= 0 {
		maxTokens = 100
	}
	if tokenRatio <= 0 {
		tokenRatio = 0.1
	}
	return &RetryBudget{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		tokenRatio: tokenRatio,
	}
}

func (b *RetryBudget) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tokens > b.maxTokens/2
}

func (b *RetryBudget) OnFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokens--
	if b.tokens < 0 {
		b.tokens = 0
	}
}

func (b *RetryBudget) OnSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokens += b.tokenRatio
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
}

func (b *RetryBudget) Tokens() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.tokens
}
