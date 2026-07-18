package fleet

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/srav-afk/forge-labs/services/routing"
)

type WorkerState string

const (
	StateProvisioning WorkerState = "provisioning"
	StateReady        WorkerState = "ready"
	StateDraining     WorkerState = "draining"
	StateRetired      WorkerState = "retired"
)

type trackedWorker struct {
	ID       WorkerID
	Model    ModelIdentity
	State    WorkerState
	InFlight int
}

type Manager struct {
	policies    *PolicyCache
	provisioner Provisioner
	lifecycle   *Lifecycle
	holder      *routing.SnapshotHolder
	rdb         *redis.Client
	metrics     *Metrics
	hyst        *Hysteresis
	interval    time.Duration

	mu      sync.Mutex
	workers map[WorkerID]*trackedWorker
	desired map[string]int
}

func NewManager(
	policies *PolicyCache,
	provisioner Provisioner,
	holder *routing.SnapshotHolder,
	rdb *redis.Client,
	metrics *Metrics,
) *Manager {
	return &Manager{
		policies:    policies,
		provisioner: provisioner,
		holder:      holder,
		rdb:         rdb,
		metrics:     metrics,
		hyst:        NewHysteresis(),
		interval:    15 * time.Second,
		workers:     map[WorkerID]*trackedWorker{},
		desired:     map[string]int{},
	}
}

func (m *Manager) SetLifecycle(l *Lifecycle) {
	m.lifecycle = l
	if l != nil {
		l.OnWorkerReady(func(wid WorkerID, id ModelIdentity) {
			m.mu.Lock()
			if w := m.workers[wid]; w != nil {
				w.State = StateReady
			}
			m.mu.Unlock()
			if m.metrics != nil {
				m.metrics.SetReady(id.Key(), m.readyCount(id))
			}
		})
	}
}

func (m *Manager) Start(ctx context.Context) {
	go m.loop(ctx)
}

func (m *Manager) loop(ctx context.Context) {
	m.ReconcileAll(ctx)
	t := time.NewTicker(m.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.ReconcileAll(ctx)
		}
	}
}

func (m *Manager) ReconcileAll(ctx context.Context) {
	_ = m.policies.Reload(ctx)
	ids := m.discoverModels()
	for _, id := range ids {
		if err := m.reconcile(ctx, id); err != nil {
			log.Printf("fleet: reconcile %s: %v", id.Key(), err)
		}
	}
}

func (m *Manager) discoverModels() []ModelIdentity {
	seen := map[string]ModelIdentity{}
	if snap := m.holder.Load(); snap != nil {
		for _, w := range snap.Workers {
			if w.BaseModel == "" {
				continue
			}
			id := ModelIdentity{BaseModel: w.BaseModel, Adapter: w.Adapter}
			seen[id.Key()] = id
		}
	}
	m.mu.Lock()
	for _, w := range m.workers {
		if w.State != StateRetired {
			seen[w.Model.Key()] = w.Model
		}
	}
	m.mu.Unlock()
	out := make([]ModelIdentity, 0, len(seen))
	for _, id := range seen {
		out = append(out, id)
	}
	return out
}

func (m *Manager) reconcile(ctx context.Context, id ModelIdentity) error {
	policy := m.policies.Get(id)
	ready := m.readyCount(id)
	active := m.sumActive(id)

	target := policy.TargetConcurrency
	if target <= 0 {
		target = 16
	}
	rawDesired := ceilDiv(active, target)
	if active > 0 && float64(active)/float64(target*max(ready, 1)) < policy.ScaleUpUtilization && rawDesired <= ready {
		rawDesired = ready
	}
	desired := clamp(rawDesired, policy.MinReplicas, policy.MaxReplicas)
	desired = m.hyst.Apply(id.Key(), desired, ready, policy, time.Now())

	m.mu.Lock()
	m.desired[id.Key()] = desired
	m.mu.Unlock()
	if m.metrics != nil {
		m.metrics.SetDesired(id.Key(), desired)
		m.metrics.SetReady(id.Key(), ready)
	}
	if m.rdb != nil {
		_ = m.rdb.Set(ctx, "fleet:desired:"+id.Key(), desired, 0).Err()
	}

	pending := m.provisioningCount(id)
	effective := ready + pending
	switch {
	case desired > effective:
		return m.scaleUp(ctx, id)
	case desired < ready:
		return m.drainOne(ctx, id)
	default:
		m.hyst.ClearPending(id.Key())
		return nil
	}
}

func (m *Manager) scaleUp(ctx context.Context, id ModelIdentity) error {
	m.mu.Lock()
	placeholder := WorkerID("provisioning-" + id.Key())
	m.workers[placeholder] = &trackedWorker{ID: placeholder, Model: id, State: StateProvisioning}
	m.mu.Unlock()

	wid, err := m.provisioner.Provision(ctx, id)
	m.mu.Lock()
	delete(m.workers, placeholder)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	m.workers[wid] = &trackedWorker{ID: wid, Model: id, State: StateProvisioning}
	m.mu.Unlock()
	if m.metrics != nil {
		m.metrics.IncScale(id.Key(), "up")
	}
	m.hyst.ClearPending(id.Key())
	log.Printf("fleet: scale up provisioned %s -> %s via %s (waiting ready)", id.Key(), wid, m.provisioner.Kind())

	if m.lifecycle != nil {
		go func() {
			wctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			if err := m.lifecycle.WaitReady(wctx, wid, id); err != nil {
				log.Printf("fleet: ready failed %s: %v (retiring)", wid, err)
				_ = m.provisioner.Retire(context.Background(), wid)
				m.mu.Lock()
				if w := m.workers[wid]; w != nil {
					w.State = StateRetired
				}
				m.mu.Unlock()
				return
			}
			m.mu.Lock()
			if w := m.workers[wid]; w != nil {
				w.State = StateReady
			}
			m.mu.Unlock()
		}()
	} else {
		m.mu.Lock()
		if w := m.workers[wid]; w != nil {
			w.State = StateReady
		}
		m.mu.Unlock()
	}
	return nil
}

func (m *Manager) drainOne(ctx context.Context, id ModelIdentity) error {
	var pick *trackedWorker
	m.mu.Lock()
	for _, w := range m.workers {
		if w.Model.Key() == id.Key() && w.State == StateReady {
			if pick == nil || w.InFlight < pick.InFlight {
				pick = w
			}
		}
	}
	if pick == nil {
		// fall back: shrink tracked ready by retiring snapshot-only surplus conceptually
		m.mu.Unlock()
		m.hyst.ClearPending(id.Key())
		return nil
	}
	pick.State = StateDraining
	wid := pick.ID
	m.mu.Unlock()

	if m.rdb != nil {
		_ = m.rdb.Set(ctx, "fleet:state:"+string(wid), string(StateDraining), 2*time.Minute).Err()
	}
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		inflight := 0
		if w := m.workers[wid]; w != nil {
			inflight = w.InFlight
		}
		m.mu.Unlock()
		if inflight <= 0 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	_ = m.provisioner.Retire(ctx, wid)
	m.mu.Lock()
	if w := m.workers[wid]; w != nil {
		w.State = StateRetired
	}
	m.mu.Unlock()
	if m.metrics != nil {
		m.metrics.IncScale(id.Key(), "down")
	}
	m.hyst.ClearPending(id.Key())
	log.Printf("fleet: scale down %s retired %s", id.Key(), wid)
	return nil
}

func (m *Manager) readyCount(id ModelIdentity) int {
	seen := map[string]struct{}{}
	if snap := m.holder.Load(); snap != nil {
		for _, w := range snap.Workers {
			if w.BaseModel == id.BaseModel && w.Adapter == id.Adapter && w.Healthy && w.Ready {
				seen[w.ID] = struct{}{}
			}
		}
	}
	m.mu.Lock()
	for _, w := range m.workers {
		if w.Model.Key() != id.Key() {
			continue
		}
		switch w.State {
		case StateReady:
			seen[string(w.ID)] = struct{}{}
		case StateProvisioning:
			// in-flight provision counts toward desired gap fill but not "ready"
		}
	}
	m.mu.Unlock()
	return len(seen)
}

func (m *Manager) provisioningCount(id ModelIdentity) int {
	n := 0
	m.mu.Lock()
	for _, w := range m.workers {
		if w.Model.Key() == id.Key() && w.State == StateProvisioning {
			n++
		}
	}
	m.mu.Unlock()
	return n
}

func (m *Manager) sumActive(id ModelIdentity) int {
	total := 0
	if snap := m.holder.Load(); snap != nil {
		seen := map[string]int{}
		for _, w := range snap.Workers {
			if w.BaseModel == id.BaseModel && w.Adapter == id.Adapter {
				seen[w.ID] = w.QueueDepth + w.InFlight
			}
		}
		for _, v := range seen {
			total += v
		}
	}
	return total
}

func (m *Manager) SetInFlight(id WorkerID, n int) {
	m.mu.Lock()
	if w := m.workers[id]; w != nil {
		w.InFlight = n
	}
	m.mu.Unlock()
}

func (m *Manager) Desired(id ModelIdentity) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.desired[id.Key()]
}

func (m *Manager) NeedsActivation(baseModel string) bool {
	id := ModelIdentity{BaseModel: baseModel}
	policy := m.policies.Get(id)
	if policy.MinReplicas > 0 {
		return false
	}
	return m.readyCount(id) == 0
}

func (m *Manager) Activate(ctx context.Context, baseModel string) error {
	id := ModelIdentity{BaseModel: baseModel}
	if m.readyCount(id) > 0 {
		return nil
	}
	return m.scaleUp(ctx, id)
}

func ceilDiv(a, b int) int {
	if b <= 0 {
		return 0
	}
	if a <= 0 {
		return 0
	}
	return (a + b - 1) / b
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if hi > 0 && v > hi {
		return hi
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
