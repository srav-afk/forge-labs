package fleet

import (
	"sync"
	"time"
)

type hysteresisState struct {
	samples    []sample
	lastAction time.Time
	pending    bool
}

type sample struct {
	at      time.Time
	desired int
}

type Hysteresis struct {
	mu   sync.Mutex
	by   map[string]*hysteresisState
	step int
}

func NewHysteresis() *Hysteresis {
	return &Hysteresis{by: map[string]*hysteresisState{}, step: 1}
}

func (h *Hysteresis) Apply(key string, desired, current int, policy ScalingPolicy, now time.Time) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.by[key]
	if st == nil {
		st = &hysteresisState{}
		h.by[key] = st
	}
	window := time.Duration(policy.StabilizationWindowSeconds) * time.Second
	if window <= 0 {
		window = 60 * time.Second
	}
	st.samples = append(st.samples, sample{at: now, desired: desired})
	cut := now.Add(-window)
	kept := st.samples[:0]
	for _, s := range st.samples {
		if s.at.After(cut) {
			kept = append(kept, s)
		}
	}
	st.samples = kept

	if st.pending {
		return current
	}

	maxD, minD := desired, desired
	for _, s := range st.samples {
		if s.desired > maxD {
			maxD = s.desired
		}
		if s.desired < minD {
			minD = s.desired
		}
	}

	out := current
	if desired > current {
		target := maxD
		out = current + h.step
		if out > target {
			out = target
		}
		st.pending = true
		st.lastAction = now
		return out
	}

	cooldown := time.Duration(policy.ScaleDownDelaySeconds) * time.Second
	if cooldown <= 0 {
		cooldown = 900 * time.Second
	}
	if desired < current && now.Sub(st.lastAction) >= cooldown {
		target := minD
		out = current - h.step
		if out < target {
			out = target
		}
		st.pending = true
		st.lastAction = now
		return out
	}
	return current
}

func (h *Hysteresis) ClearPending(key string) {
	h.mu.Lock()
	if st := h.by[key]; st != nil {
		st.pending = false
	}
	h.mu.Unlock()
}
