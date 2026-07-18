package fleet

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type WorkerID string

type Provisioner interface {
	Provision(ctx context.Context, id ModelIdentity) (WorkerID, error)
	Retire(ctx context.Context, w WorkerID) error
	Kind() string
}

type LocalProcess struct {
	mu      sync.Mutex
	next    int
	active  map[WorkerID]ModelIdentity
	onReady func(WorkerID, ModelIdentity)
	onGone  func(WorkerID)
}

func NewLocalProcess() *LocalProcess {
	return &LocalProcess{active: map[WorkerID]ModelIdentity{}}
}

func (p *LocalProcess) Kind() string { return "local" }

func (p *LocalProcess) OnReady(fn func(WorkerID, ModelIdentity)) { p.onReady = fn }
func (p *LocalProcess) OnGone(fn func(WorkerID))                 { p.onGone = fn }

func (p *LocalProcess) Provision(ctx context.Context, id ModelIdentity) (WorkerID, error) {
	_ = ctx
	p.mu.Lock()
	defer p.mu.Unlock()
	p.next++
	wid := WorkerID(fmt.Sprintf("local-%s-%d", id.Key(), p.next))
	p.active[wid] = id
	if p.onReady != nil {
		go p.onReady(wid, id)
	}
	return wid, nil
}

func (p *LocalProcess) Retire(ctx context.Context, w WorkerID) error {
	_ = ctx
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.active, w)
	if p.onGone != nil {
		go p.onGone(w)
	}
	return nil
}

func (p *LocalProcess) Active() map[WorkerID]ModelIdentity {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[WorkerID]ModelIdentity, len(p.active))
	for k, v := range p.active {
		out[k] = v
	}
	return out
}

type RunPodProvisioner struct {
	enabled bool
	mu      sync.Mutex
	pods    map[WorkerID]time.Time
}

func NewRunPodProvisioner(enabled bool) *RunPodProvisioner {
	return &RunPodProvisioner{enabled: enabled, pods: map[WorkerID]time.Time{}}
}

func (p *RunPodProvisioner) Kind() string { return "runpod" }

func (p *RunPodProvisioner) Provision(ctx context.Context, id ModelIdentity) (WorkerID, error) {
	if !p.enabled {
		return "", fmt.Errorf("runpod provisioner disabled (set FORGE_FLEET_RUNPOD_ENABLED=true)")
	}
	_ = ctx
	p.mu.Lock()
	defer p.mu.Unlock()
	wid := WorkerID(fmt.Sprintf("runpod-%s-%d", id.Key(), time.Now().UnixNano()))
	p.pods[wid] = time.Now().UTC()
	return wid, nil
}

func (p *RunPodProvisioner) Retire(ctx context.Context, w WorkerID) error {
	_ = ctx
	p.mu.Lock()
	delete(p.pods, w)
	p.mu.Unlock()
	return nil
}
