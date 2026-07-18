# RunPod + Forge (fleet provisioner)

## Modes

### A. Automated (cp-15 path)

Control plane can create/delete GPU pods via RunPod REST when:

```bash
export FORGE_FLEET_RUNPOD_ENABLED=true
export FORGE_FLEET_RUNPOD_DRY_RUN=false   # default is true (safe)
export FORGE_RUNPOD_API_KEY=...
export FORGE_HF_TOKEN=...
export FORGE_FLEET_RUNPOD_GPU_TYPE="NVIDIA GeForce RTX 4090"
export FORGE_FLEET_RUNPOD_IMAGE=vllm/vllm-openai:latest
export FORGE_FLEET_RUNPOD_VLLM_MODEL=Qwen/Qwen2.5-0.5B-Instruct
export FORGE_FLEET_RUNPOD_CLOUD_TYPE=COMMUNITY
```

Fleet scale-up calls `POST https://rest.runpod.io/v1/pods` and starts vLLM OpenAI server on port 8000.
Proxy URL: `https://<pod-id>-8000.proxy.runpod.net`

**Important:** gRPC `forge-worker` on the pod still needs network reachability (Tailscale). The HTTP OpenAI port is usable as a **provider** base URL without Tailscale:

```bash
FORGE_ENV_FILE=development.tier-a.env go run ./cmd/forge-catalog provider-upsert \
  runpod-vllm "https://PODID-8000.proxy.runpod.net/v1" FORGE_RUNPOD_API_KEY \
  Qwen/Qwen2.5-0.5B-Instruct=Qwen/Qwen2.5-0.5B-Instruct
```

(If the proxy needs no API key, set a dummy `api_key_ref` or empty-key provider path.)

Default **dry-run** avoids accidental spend. Account balance must cover community GPU time.

### B. Manual worker (full gRPC path)

1. Create pod (template or image) with GPU.
2. Install Tailscale; set worker endpoint to `100.x:50051`.
3. Run vLLM + `forge-worker` with `RUNTIME_KIND_VLLM`.

See historical model notes below.

## Model choices (as of 2026-07-18)

| Role | Id |
|------|----|
| Cheap GPU test | `Qwen/Qwen2.5-0.5B-Instruct` |
| Local free | `qwen3.5:0.8b` / `qwen3.6:27b` |
| Big SaaS | Baseten `nvidia/Nemotron-120B-A12B` |

## Teardown

```bash
runpodctl pod list --all
runpodctl pod delete <id>
# or fleet scale-down / provisioner Retire
```

Never leave GPUs idle with dry-run=false.
