# RunPod vLLM worker (manual, cp-13)

cp-13 does **not** provision pods. Rent a GPU, start vLLM + Tailscale + `forge-worker`, and it registers like any other worker. Automated create/drain is cp-15.

## Model choices (keep current)

Examples below use models people actually run in mid-2026 — not Llama 3.1 / Qwen2.5 era defaults:

| Role | Example id | Why |
|------|------------|-----|
| Free Mac (Ollama) | `qwen3:8b` | small, local, `$0/hr` |
| Paid GPU (vLLM) | `Qwen/Qwen3-235B-A22B` | large MoE; needs serious VRAM / multi-GPU |
| Alt large | `meta-llama/Llama-4-Scout-17B-16E` | long-context MoE if you prefer Meta |

Pick whatever you pull; Forge routes on the **string** you register as `worker.model.base`.

## 1. Pod

- GPU: enough for the large model (multi-A100/H100 for 235B-class; smaller SKU for Scout-class)
- Template: PyTorch or bare Ubuntu
- Expose **no public ports** — only Tailscale

## 2. Tailscale (ephemeral)

On the pod:

```bash
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up --authkey=tskey-auth-XXXX --hostname=forge-runpod-a --advertise-tags=tag:forge
tailscale ip -4   # e.g. 100.81.4.12
```

Use an **ephemeral** auth key so a destroyed pod leaves the tailnet.

## 3. vLLM

```bash
export MODEL=Qwen/Qwen3-235B-A22B
python -m vllm.entrypoints.openai.api_server \
  --model "$MODEL" \
  --host 127.0.0.1 \
  --port 8000 \
  --max-model-len 131072
```

Metrics: `http://127.0.0.1:8000/metrics` (`vllm:num_requests_running`, `vllm:kv_cache_usage_perc`, …).

## 4. forge-worker on the pod

```bash
export FORGE_WORKER_ID=pod-a
export FORGE_WORKER_ENDPOINT=100.81.4.12:50051
export FORGE_WORKER_GRPC_ADDR=:50051
export FORGE_WORKER_RUNTIME=RUNTIME_KIND_VLLM
export FORGE_WORKER_MODEL_BASE=Qwen/Qwen3-235B-A22B
export FORGE_WORKER_MODEL_CONTEXT=131072
export FORGE_VLLM_URL=http://127.0.0.1:8000
export FORGE_VLLM_SERVED_MODEL=Qwen/Qwen3-235B-A22B
export FORGE_WORKER_COST_PER_HOUR=1.19
export FORGE_WORKER_COST_CLASS=paid
export FORGE_CONTROLPLANE_GRPC=100.x.y.z:8081
export FORGE_REDIS_URL=redis://100.x.y.z:6379/0

export FORGE_WORKER_CAPABILITIES_RUNTIME=vllm
export FORGE_WORKER_CAPABILITIES_GPU=H100
export FORGE_WORKER_CAPABILITIES_VRAM_GB=80
export FORGE_WORKER_CAPABILITIES_COST_PER_HOUR=1.19
export FORGE_WORKER_CAPABILITIES_COST_CLASS=paid
export FORGE_WORKER_CAPABILITIES_MAX_MODEL_LEN=131072

./forge-worker
```

Canonical Forge id is `FORGE_WORKER_MODEL_BASE`. For a short catalog name (`qwen3-235b-a22b`), set that as base and keep `FORGE_VLLM_SERVED_MODEL` as the HF path.

## 5. Local Mac worker (free)

```bash
export FORGE_WORKER_ID=mac-1
export FORGE_WORKER_RUNTIME=RUNTIME_KIND_OLLAMA
export FORGE_WORKER_MODEL_BASE=qwen3:8b
export FORGE_WORKER_COST_PER_HOUR=0
export FORGE_WORKER_COST_CLASS=free
./forge-worker
```

## 6. Routing check

```bash
curl -s localhost:8080/v1/chat/completions -H 'content-type: application/json' \
  -d '{"model":"qwen3:8b","messages":[{"role":"user","content":"hi"}]}'

curl -s localhost:8080/v1/chat/completions -H 'content-type: application/json' \
  -d '{"model":"Qwen/Qwen3-235B-A22B","messages":[{"role":"user","content":"hi"}]}'
```

## 7. Teardown

Stop the pod. Within heartbeat TTL (~6s) + reconcile it drops from the snapshot; large-model requests then 503 `no_capacity` (or catalog 404 if unassigned).
