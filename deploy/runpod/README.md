# RunPod vLLM worker (manual, cp-13)

cp-13 does **not** provision pods. Rent a GPU, start vLLM + Tailscale + `forge-worker`, and it registers like any other worker. Automated create/drain is cp-15.

## 1. Pod

- GPU: A100 80GB (or similar) for 70B
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
export MODEL=meta-llama/Llama-3.1-70B-Instruct
python -m vllm.entrypoints.openai.api_server \
  --model "$MODEL" \
  --host 127.0.0.1 \
  --port 8000 \
  --max-model-len 131072
```

Metrics: `http://127.0.0.1:8000/metrics` (`vllm:num_requests_running`, `vllm:kv_cache_usage_perc`, …).

## 4. forge-worker on the pod

Point the control plane at a **tailnet** address (or reverse tunnel). Worker endpoint is the pod’s Tailscale IP.

```bash
export FORGE_WORKER_ID=pod-a
export FORGE_WORKER_ENDPOINT=100.81.4.12:50051
export FORGE_WORKER_GRPC_ADDR=:50051
export FORGE_WORKER_RUNTIME=RUNTIME_KIND_VLLM
export FORGE_WORKER_MODEL_BASE=meta-llama/Llama-3.1-70B-Instruct
export FORGE_WORKER_MODEL_CONTEXT=131072
export FORGE_VLLM_URL=http://127.0.0.1:8000
export FORGE_VLLM_SERVED_MODEL=meta-llama/Llama-3.1-70B-Instruct
export FORGE_WORKER_COST_PER_HOUR=1.19
export FORGE_WORKER_COST_CLASS=paid
export FORGE_CONTROLPLANE_GRPC=100.x.y.z:8081   # Mac control plane on tailnet
export FORGE_REDIS_URL=redis://100.x.y.z:6379/0

# optional capability map overrides (JSON-ish via env is limited; defaults merge cost/runtime)
export FORGE_WORKER_CAPABILITIES_RUNTIME=vllm
export FORGE_WORKER_CAPABILITIES_GPU=A100-80GB
export FORGE_WORKER_CAPABILITIES_VRAM_GB=80
export FORGE_WORKER_CAPABILITIES_COST_PER_HOUR=1.19
export FORGE_WORKER_CAPABILITIES_COST_CLASS=paid
export FORGE_WORKER_CAPABILITIES_MAX_MODEL_LEN=131072

./forge-worker
```

Canonical model id in Forge is `FORGE_WORKER_MODEL_BASE`. If you want a short catalog name (`llama-3.1-70b`), set that as base and `FORGE_VLLM_SERVED_MODEL` to the HF path.

## 5. Local Mac worker (free)

```bash
export FORGE_WORKER_ID=mac-1
export FORGE_WORKER_RUNTIME=RUNTIME_KIND_OLLAMA
export FORGE_WORKER_MODEL_BASE=qwen2.5-7b
export FORGE_WORKER_COST_PER_HOUR=0
export FORGE_WORKER_COST_CLASS=free
./forge-worker
```

## 6. Routing check

```bash
# free path
curl -s localhost:8080/v1/chat/completions -H 'content-type: application/json' \
  -d '{"model":"qwen2.5-7b","messages":[{"role":"user","content":"hi"}]}'

# paid GPU path (only if catalog/registry has the 70B on pod-a)
curl -s localhost:8080/v1/chat/completions -H 'content-type: application/json' \
  -d '{"model":"meta-llama/Llama-3.1-70B-Instruct","messages":[{"role":"user","content":"hi"}]}'
```

Traces: chosen worker id on `scheduler.rank`. Grafana: both workers’ metrics; pod shows scraped vLLM load via heartbeat `inflight`/`queue_depth`.

## 7. Teardown

Stop the pod. Within heartbeat TTL (~6s) + reconcile, it drops from the routing snapshot. 70B requests then 503 `no_capacity` (or `model_not_found` if catalog still lists only that assignee).
