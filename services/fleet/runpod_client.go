package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const runpodREST = "https://rest.runpod.io/v1"

type RunPodClient struct {
	apiKey string
	http   *http.Client
	base   string
}

func NewRunPodClient(apiKey string) *RunPodClient {
	if apiKey == "" {
		apiKey = os.Getenv("FORGE_RUNPOD_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("RUNPOD_API_KEY")
	}
	return &RunPodClient{
		apiKey: apiKey,
		http:   &http.Client{Timeout: 60 * time.Second},
		base:   runpodREST,
	}
}

type runPodCreateRequest struct {
	Name              string            `json:"name"`
	ImageName         string            `json:"imageName"`
	GpuTypeIds        []string          `json:"gpuTypeIds"`
	GpuCount          int               `json:"gpuCount"`
	CloudType         string            `json:"cloudType,omitempty"`
	ContainerDiskInGb int               `json:"containerDiskInGb"`
	VolumeInGb        int               `json:"volumeInGb"`
	VolumeMountPath   string            `json:"volumeMountPath,omitempty"`
	Ports             []string          `json:"ports,omitempty"`
	Env               map[string]string `json:"env,omitempty"`
	DockerStartCmd    []string          `json:"dockerStartCmd,omitempty"`
	DockerArgs        string            `json:"dockerArgs,omitempty"`
	SupportPublicIp   bool              `json:"supportPublicIp,omitempty"`
	MinVcpuCount      int               `json:"minVcpuCount,omitempty"`
	MinMemoryInGb     int               `json:"minMemoryInGb,omitempty"`
}

type RunPodPod struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DesiredStatus string `json:"desiredStatus"`
	ImageName     string `json:"imageName"`
	MachineID     string `json:"machineId"`
	PublicIP      string `json:"publicIp"`
}

func (c *RunPodClient) CreatePod(ctx context.Context, req runPodCreateRequest) (*RunPodPod, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("runpod: missing API key (FORGE_RUNPOD_API_KEY)")
	}
	if req.GpuCount <= 0 {
		req.GpuCount = 1
	}
	if req.ContainerDiskInGb <= 0 {
		req.ContainerDiskInGb = 40
	}
	if req.VolumeInGb < 0 {
		req.VolumeInGb = 0
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/pods", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("runpod create pod: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var pod RunPodPod
	if err := json.Unmarshal(raw, &pod); err != nil {
		var wrap struct {
			ID string `json:"id"`
		}
		if err2 := json.Unmarshal(raw, &wrap); err2 != nil || wrap.ID == "" {
			return nil, fmt.Errorf("runpod create pod: decode: %w body=%s", err, truncate(string(raw), 400))
		}
		pod.ID = wrap.ID
	}
	if pod.ID == "" {
		return nil, fmt.Errorf("runpod create pod: empty id body=%s", truncate(string(raw), 400))
	}
	return &pod, nil
}

func (c *RunPodClient) GetPod(ctx context.Context, id string) (*RunPodPod, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/pods/"+id, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("runpod get pod: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var pod RunPodPod
	if err := json.Unmarshal(raw, &pod); err != nil {
		return nil, err
	}
	return &pod, nil
}

func (c *RunPodClient) StopPod(ctx context.Context, id string) error {
	return c.podAction(ctx, id, "stop")
}

func (c *RunPodClient) DeletePod(ctx context.Context, id string) error {
	if c.apiKey == "" {
		return fmt.Errorf("runpod: missing API key")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.base+"/pods/"+id, nil)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("runpod delete pod: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func (c *RunPodClient) podAction(ctx context.Context, id, action string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/pods/"+id+"/"+action, nil)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("runpod %s pod: status %d: %s", action, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func (c *RunPodClient) ProxyURL(podID string, port int) string {
	return fmt.Sprintf("https://%s-%d.proxy.runpod.net", podID, port)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
