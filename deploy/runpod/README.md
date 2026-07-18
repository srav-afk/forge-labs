# RunPod vLLM worker (manual, cp-13)

cp-13 does **not** provision pods. Rent a GPU, start vLLM + Tailscale + `forge-worker`, and it registers like any other worker. Automated create/drain is cp-15.

## Model choices (as of 2026-07-18)

Pulled from Hugging Face model cards + Ollama library (newest tags), not plan-doc nostalgia:

| Role | Id | Why (sources) |
|------|----|----------------|
| Free Mac (Ollama) | `qwen3.6:27b` | Latest open Qwen dense that people self-host; Ollama `qwen3.6` (27b/35b). HF: [`Qwen/Qwen3.6-27B`](https://huggingface.co/Qwen/Qwen3.6-27B) |
| Alt local | `gemma4:12b` | Gemma 4 family; Ollama updated ~2 weeks ago |
| Paid GPU (vLLM) | `zai-org/GLM-5.2` | Current open-weight coding/agent flagship (~753B MoE, MIT, 1M ctx). HF: [`zai-org/GLM-5.2`](https://huggingface.co/zai-org/GLM-5.2); vLLM ≥0.23 |
| Alt large | `moonshotai/Kimi-K2.7-Code` or `deepseek-ai/DeepSeek-V4-Flash` | June 2026 agent/coding contenders |

Qwen3.7-Max is API-only as of mid-2026 — no open weights. Llama 4 is deprioritized in community self-host roundups; not used as the default here.

Forge routes on the **string** you register as `worker.model.base`.

## 1. Pod

- GPU: multi-H100/H200 class for `GLM-5.2` (not a single 24GB card)
- Template: PyTorch or bare Ubuntu
- Expose **no public ports** — only Tailscale

## 2. Tailscale (ephemeral)

```bash
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up --authkey=tskey-auth-XXXX --hostname=forge-runpod-a --advertise-tags=tag:forge
tailscale ip -4
```

## 3. vLLM

```bash
export MODEL=zai-org/GLM-5.2
# vllm>=0.23.0 per model card
vllm serve "$MODEL" \
  --host 127.0.0.1 \
  --port 8000 \
  --max-model-len 1048576
```

Metrics: `http://127.0.0.1:8000/metrics`.

## 4. forge-worker on the pod

```bash
export FORGE_WORKER_ID=pod-a
export FORGE_WORKER_ENDPOINT=100.81.4.12:50051
export FORGE_WORKER_GRPC_ADDR=:50051
export FORGE_WORKER_RUNTIME=RUNTIME_KIND_VLLM
export FORGE_WORKER_MODEL_BASE=zai-org/GLM-5.2
export FORGE_WORKER_MODEL_CONTEXT=1048576
export FORGE_VLLM_URL=http://127.0.0.1:8000
export FORGE_VLLM_SERVED_MODEL=zai-org/GLM-5.2
export FORGE_WORKER_COST_PER_HOUR=1.19
export FORGE_WORKER_COST_CLASS=paid
export FORGE_CONTROLPLANE_GRPC=100.x.y.z:8081
export FORGE_REDIS_URL=redis://100.x.y.z:6379/0

export FORGE_WORKER_CAPABILITIES_RUNTIME=vllm
export FORGE_WORKER_CAPABILITIES_GPU=H100
export FORGE_WORKER_CAPABILITIES_VRAM_GB=80
export FORGE_WORKER_CAPABILITIES_COST_PER_HOUR=1.19
export FORGE_WORKER_CAPABILITIES_COST_CLASS=paid
export FORGE_WORKER_CAPABILITIES_MAX_MODEL_LEN=1048576

./forge-worker
```

Short catalog name: set `FORGE_WORKER_MODEL_BASE=glm-5.2` and keep `FORGE_VLLM_SERVED_MODEL=zai-org/GLM-5.2`.

## 5. Local Mac worker (free)

```bash
ollama pull qwen3.6:27b
export FORGE_WORKER_ID=mac-1
export FORGE_WORKER_RUNTIME=RUNTIME_KIND_OLLAMA
export FORGE_WORKER_MODEL_BASE=qwen3.6:27b
export FORGE_WORKER_MODEL_CONTEXT=262144
export FORGE_WORKER_COST_PER_HOUR=0
export FORGE_WORKER_COST_CLASS=free
./forge-worker
```

## 6. Routing check

```bash
curl -s localhost:8080/v1/chat/completions -H 'content-type: application/json' \
  -d '{"model":"qwen3.6:27b","messages":[{"role":"user","content":"hi"}]}'

curl -s localhost:8080/v1/chat/completions -H 'content-type: application/json' \
  -d '{"model":"zai-org/GLM-5.2","messages":[{"role":"user","content":"hi"}]}'
```

## 7. Teardown

Stop the pod. Within heartbeat TTL (~6s) + reconcile it drops from the snapshot.
