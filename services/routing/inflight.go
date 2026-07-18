package routing

import (
	"sync"
	"sync/atomic"
)

type InflightTracker struct {
	m sync.Map
}

func NewInflightTracker() *InflightTracker {
	return &InflightTracker{}
}

func (t *InflightTracker) counter(workerID string) *atomic.Int64 {
	if v, ok := t.m.Load(workerID); ok {
		return v.(*atomic.Int64)
	}
	c := &atomic.Int64{}
	actual, _ := t.m.LoadOrStore(workerID, c)
	return actual.(*atomic.Int64)
}

func (t *InflightTracker) Inc(workerID string) {
	t.counter(workerID).Add(1)
}

func (t *InflightTracker) Dec(workerID string) {
	c := t.counter(workerID)
	for {
		cur := c.Load()
		if cur <= 0 {
			c.Store(0)
			return
		}
		if c.CompareAndSwap(cur, cur-1) {
			return
		}
	}
}

func (t *InflightTracker) Get(workerID string) int {
	if v, ok := t.m.Load(workerID); ok {
		return int(v.(*atomic.Int64).Load())
	}
	return 0
}

func (t *InflightTracker) Track(workerID string) (done func()) {
	t.Inc(workerID)
	var once sync.Once
	return func() {
		once.Do(func() { t.Dec(workerID) })
	}
}

func (t *InflightTracker) TryTrack(workerID string, limit int64) (done func(), ok bool) {
	if limit <= 0 {
		return t.Track(workerID), true
	}
	c := t.counter(workerID)
	for {
		cur := c.Load()
		if cur >= limit {
			return nil, false
		}
		if c.CompareAndSwap(cur, cur+1) {
			var once sync.Once
			return func() {
				once.Do(func() { t.Dec(workerID) })
			}, true
		}
	}
}
