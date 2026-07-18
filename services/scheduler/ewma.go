package scheduler

import (
	"math"
	"time"
)

type EWMA struct {
	tau   time.Duration
	value float64
	last  time.Time
}

func NewEWMA(tau time.Duration, seed float64) *EWMA {
	if tau <= 0 {
		tau = 10 * time.Second
	}
	return &EWMA{tau: tau, value: seed}
}

func (e *EWMA) Update(sampleMs float64, now time.Time) {
	if e.last.IsZero() {
		e.value = sampleMs
		e.last = now
		return
	}
	dt := now.Sub(e.last).Seconds()
	if dt < 0 {
		dt = 0
	}
	tauSec := e.tau.Seconds()
	if tauSec <= 0 {
		tauSec = 10
	}
	alpha := 1 - math.Exp(-dt/tauSec)
	e.value = alpha*sampleMs + (1-alpha)*e.value
	e.last = now
}

func (e *EWMA) Value() float64 { return e.value }
