package fleet

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type RunPodProvisionerConfig struct {
	Enabled        bool
	DryRun         bool
	APIKey         string
	GPUTypeID      string
	Image          string
	CloudType      string
	ContainerDisk  int
	VolumeDisk     int
	HFToken        string
	VLLMModel      string
	VLLMPort       int
	ControlPlane   string
	RedisURL       string
	WorkerBinaryHint string
}

type runpodRecord struct {
	PodID     string
	Model     ModelIdentity
	CreatedAt time.Time
	ProxyURL  string
}

type RunPodProvisioner struct {
	cfg    RunPodProvisionerConfig
	client *RunPodClient
	mu     sync.Mutex
	pods   map[WorkerID]runpodRecord
}

func NewRunPodProvisioner(cfg RunPodProvisionerConfig) *RunPodProvisioner {
	if cfg.GPUTypeID == "" {
		cfg.GPUTypeID = "NVIDIA GeForce RTX 4090"
	}
	if cfg.Image == "" {
		cfg.Image = "vllm/vllm-openai:latest"
	}
	if cfg.CloudType == "" {
		cfg.CloudType = "COMMUNITY"
	}
	if cfg.ContainerDisk <= 0 {
		cfg.ContainerDisk = 50
	}
	if cfg.VLLMPort <= 0 {
		cfg.VLLMPort = 8000
	}
	if cfg.APIKey == "" {
		cfg.APIKey = os.Getenv("FORGE_RUNPOD_API_KEY")
		if cfg.APIKey == "" {
			cfg.APIKey = os.Getenv("RUNPOD_API_KEY")
		}
	}
	if cfg.HFToken == "" {
		cfg.HFToken = os.Getenv("FORGE_HF_TOKEN")
		if cfg.HFToken == "" {
			cfg.HFToken = os.Getenv("HF_TOKEN")
		}
	}
	return &RunPodProvisioner{
		cfg:    cfg,
		client: NewRunPodClient(cfg.APIKey),
		pods:   map[WorkerID]runpodRecord{},
	}
}

func (p *RunPodProvisioner) Kind() string { return "runpod" }

func (p *RunPodProvisioner) Provision(ctx context.Context, id ModelIdentity) (WorkerID, error) {
	if !p.cfg.Enabled {
		return "", fmt.Errorf("runpod provisioner disabled (set FORGE_FLEET_RUNPOD_ENABLED=true)")
	}
	model := id.BaseModel
	if p.cfg.VLLMModel != "" {
		model = p.cfg.VLLMModel
	}
	if model == "" {
		return "", fmt.Errorf("runpod: empty model identity")
	}

	if p.cfg.DryRun {
		wid := WorkerID(fmt.Sprintf("runpod-dryrun-%s-%d", sanitizeID(id.Key()), time.Now().UnixNano()))
		p.mu.Lock()
		p.pods[wid] = runpodRecord{PodID: "dryrun", Model: id, CreatedAt: time.Now().UTC()}
		p.mu.Unlock()
		log.Printf("fleet/runpod: dry-run provision %s -> %s", id.Key(), wid)
		return wid, nil
	}

	name := fmt.Sprintf("forge-%s-%d", sanitizeID(id.Key()), time.Now().Unix()%100000)
	env := map[string]string{
		"HF_TOKEN":                 p.cfg.HFToken,
		"HUGGING_FACE_HUB_TOKEN":   p.cfg.HFToken,
		"FORGE_MODEL":              model,
		"FORGE_VLLM_PORT":          fmt.Sprintf("%d", p.cfg.VLLMPort),
		"FORGE_CONTROLPLANE_GRPC":  p.cfg.ControlPlane,
		"FORGE_REDIS_URL":          p.cfg.RedisURL,
	}
	startCmd := []string{
		"bash", "-lc",
		fmt.Sprintf(
			`set -euo pipefail
export HF_HOME=/workspace/hf
mkdir -p /workspace/hf
python3 -m vllm.entrypoints.openai.api_server \
  --model %q \
  --host 0.0.0.0 \
  --port %d \
  --max-model-len 8192 \
  --dtype auto
`, model, p.cfg.VLLMPort),
	}

	req := runPodCreateRequest{
		Name:              name,
		ImageName:         p.cfg.Image,
		GpuTypeIds:        []string{p.cfg.GPUTypeID},
		GpuCount:          1,
		CloudType:         p.cfg.CloudType,
		ContainerDiskInGb: p.cfg.ContainerDisk,
		VolumeInGb:        p.cfg.VolumeDisk,
		VolumeMountPath:   "/workspace",
		Ports:             []string{fmt.Sprintf("%d/http", p.cfg.VLLMPort), "22/tcp"},
		Env:               env,
		DockerStartCmd:    startCmd,
		SupportPublicIp:   false,
	}

	pod, err := p.client.CreatePod(ctx, req)
	if err != nil {
		return "", err
	}
	wid := WorkerID("runpod-" + pod.ID)
	proxy := p.client.ProxyURL(pod.ID, p.cfg.VLLMPort)
	p.mu.Lock()
	p.pods[wid] = runpodRecord{
		PodID:     pod.ID,
		Model:     id,
		CreatedAt: time.Now().UTC(),
		ProxyURL:  proxy,
	}
	p.mu.Unlock()
	log.Printf("fleet/runpod: created pod %s worker=%s proxy=%s model=%s", pod.ID, wid, proxy, model)
	return wid, nil
}

func (p *RunPodProvisioner) Retire(ctx context.Context, w WorkerID) error {
	p.mu.Lock()
	rec, ok := p.pods[w]
	if ok {
		delete(p.pods, w)
	}
	p.mu.Unlock()
	if !ok {
		if strings.HasPrefix(string(w), "runpod-") {
			id := strings.TrimPrefix(string(w), "runpod-")
			if id != "" && id != "dryrun" && !p.cfg.DryRun {
				return p.client.DeletePod(ctx, id)
			}
		}
		return nil
	}
	if rec.PodID == "" || rec.PodID == "dryrun" || p.cfg.DryRun {
		return nil
	}
	if err := p.client.DeletePod(ctx, rec.PodID); err != nil {
		log.Printf("fleet/runpod: delete %s: %v", rec.PodID, err)
		return err
	}
	log.Printf("fleet/runpod: deleted pod %s", rec.PodID)
	return nil
}

func (p *RunPodProvisioner) Pod(w WorkerID) (runpodRecord, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r, ok := p.pods[w]
	return r, ok
}

func sanitizeID(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ":", "-")
	s = strings.ReplaceAll(s, "#", "-")
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}
