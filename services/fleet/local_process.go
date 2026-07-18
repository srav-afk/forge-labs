package fleet

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type LocalProcessConfig struct {
	Binary       string
	EnvFile      string
	BaseGRPCPort int
	ControlPlane string
	RedisURL     string
	DBURL        string
	OllamaURL    string
	TrackOnly    bool
}

type localProc struct {
	cmd    *exec.Cmd
	model  ModelIdentity
	port   int
	cancel context.CancelFunc
}

type LocalProcess struct {
	cfg     LocalProcessConfig
	mu      sync.Mutex
	next    int
	active  map[WorkerID]*localProc
	onReady func(WorkerID, ModelIdentity)
	onGone  func(WorkerID)
}

func NewLocalProcess() *LocalProcess {
	return NewLocalProcessWithConfig(LocalProcessConfig{})
}

func NewLocalProcessWithConfig(cfg LocalProcessConfig) *LocalProcess {
	if cfg.BaseGRPCPort <= 0 {
		cfg.BaseGRPCPort = 50100
	}
	if cfg.Binary == "" {
		if b := os.Getenv("FORGE_WORKER_BINARY"); b != "" {
			cfg.Binary = b
		} else if _, err := os.Stat("bin/forge-worker"); err == nil {
			cfg.Binary = "bin/forge-worker"
		} else {
			cfg.Binary = "forge-worker"
		}
	}
	if cfg.OllamaURL == "" {
		cfg.OllamaURL = "http://127.0.0.1:11434"
	}
	return &LocalProcess{
		cfg:    cfg,
		active: map[WorkerID]*localProc{},
	}
}

func (p *LocalProcess) Kind() string { return "local" }

func (p *LocalProcess) OnReady(fn func(WorkerID, ModelIdentity)) { p.onReady = fn }
func (p *LocalProcess) OnGone(fn func(WorkerID))                 { p.onGone = fn }

func (p *LocalProcess) Provision(ctx context.Context, id ModelIdentity) (WorkerID, error) {
	_ = ctx
	p.mu.Lock()
	p.next++
	n := p.next
	p.mu.Unlock()
	wid := WorkerID(fmt.Sprintf("local-%s-%d", sanitizeID(id.Key()), n))

	if p.cfg.TrackOnly {
		p.mu.Lock()
		p.active[wid] = &localProc{model: id}
		p.mu.Unlock()
		if p.onReady != nil {
			go p.onReady(wid, id)
		}
		return wid, nil
	}

	port, err := freePortNear(p.cfg.BaseGRPCPort)
	if err != nil {
		return "", err
	}
	bin, err := exec.LookPath(p.cfg.Binary)
	if err != nil {
		abs, err2 := filepath.Abs(p.cfg.Binary)
		if err2 != nil {
			return "", fmt.Errorf("local provisioner: worker binary %q: %w", p.cfg.Binary, err)
		}
		bin = abs
	}

	env := os.Environ()
	env = append(env,
		"FORGE_WORKER_ID="+string(wid),
		fmt.Sprintf("FORGE_WORKER_ENDPOINT=127.0.0.1:%d", port),
		fmt.Sprintf("FORGE_WORKER_GRPC_ADDR=:%d", port),
		fmt.Sprintf("FORGE_METRICS_ADDR=:%d", port+1000),
		"FORGE_WORKER_RUNTIME=RUNTIME_KIND_OLLAMA",
		"FORGE_WORKER_MODEL_BASE="+id.BaseModel,
		"FORGE_WORKER_MODEL_CONTEXT=32768",
		"FORGE_WORKER_COST_PER_HOUR=0",
		"FORGE_WORKER_COST_CLASS=free",
		"FORGE_OLLAMA_URL="+p.cfg.OllamaURL,
		"FORGE_OTLP_ENDPOINT=",
	)
	if p.cfg.ControlPlane != "" {
		env = append(env, "FORGE_CONTROLPLANE_GRPC="+p.cfg.ControlPlane)
	}
	if p.cfg.RedisURL != "" {
		env = append(env, "FORGE_REDIS_URL="+p.cfg.RedisURL)
	}
	if p.cfg.DBURL != "" {
		env = append(env, "FORGE_DB_URL="+p.cfg.DBURL)
	}
	if p.cfg.EnvFile != "" {
		env = append(env, "FORGE_ENV_FILE="+p.cfg.EnvFile)
	}

	cmdCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(cmdCtx, bin)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		cancel()
		return "", fmt.Errorf("local provisioner start: %w", err)
	}

	p.mu.Lock()
	p.active[wid] = &localProc{cmd: cmd, model: id, port: port, cancel: cancel}
	p.mu.Unlock()

	go func() {
		_ = cmd.Wait()
		p.mu.Lock()
		delete(p.active, wid)
		p.mu.Unlock()
		if p.onGone != nil {
			p.onGone(wid)
		}
		log.Printf("fleet/local: worker %s exited", wid)
	}()

	if p.onReady != nil {
		go p.onReady(wid, id)
	}
	log.Printf("fleet/local: provisioned %s model=%s grpc=:%d pid=%d", wid, id.Key(), port, cmd.Process.Pid)
	return wid, nil
}

func (p *LocalProcess) Retire(ctx context.Context, w WorkerID) error {
	_ = ctx
	p.mu.Lock()
	proc, ok := p.active[w]
	if ok {
		delete(p.active, w)
	}
	p.mu.Unlock()
	if !ok {
		return nil
	}
	if proc.cancel != nil {
		proc.cancel()
	}
	if proc.cmd != nil && proc.cmd.Process != nil {
		_ = proc.cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_, _ = proc.cmd.Process.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = proc.cmd.Process.Kill()
		}
	}
	if p.onGone != nil {
		go p.onGone(w)
	}
	log.Printf("fleet/local: retired %s", w)
	return nil
}

func (p *LocalProcess) Active() map[WorkerID]ModelIdentity {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[WorkerID]ModelIdentity, len(p.active))
	for k, v := range p.active {
		out[k] = v.model
	}
	return out
}

func freePortNear(start int) (int, error) {
	for p := start; p < start+200; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return p, nil
	}
	return 0, fmt.Errorf("no free port near %d", start)
}
