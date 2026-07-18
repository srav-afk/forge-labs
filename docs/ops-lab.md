# Ops lab — run everything except Forge Runtime

## Secrets (never commit)

```bash
# development.secrets.env (gitignored)
FORGE_BASETEN_API_KEY=...
FORGE_HF_TOKEN=...
FORGE_RUNPOD_API_KEY=...
```

## One-shot local lab

```bash
./scripts/lab-up.sh
./scripts/lab-down.sh
```

Uses `development.tier-a.env` + compose project `forge-tier-a` (Postgres `25432`, Redis `26379`).

## Catalog CLI

```bash
FORGE_ENV_FILE=development.tier-a.env go run ./cmd/forge-catalog models
FORGE_ENV_FILE=development.tier-a.env go run ./cmd/forge-catalog assign qwen3.5:0.8b mac-tier-a-01
FORGE_ENV_FILE=development.tier-a.env go run ./cmd/forge-catalog fleet-policy qwen3.5:0.8b 1 3 4
FORGE_ENV_FILE=development.tier-a.env go run ./cmd/forge-catalog provider-upsert baseten \
  https://inference.baseten.co/v1 FORGE_BASETEN_API_KEY \
  nvidia/Nemotron-120B-A12B=nvidia/Nemotron-120B-A12B
```

## Cache registry (manual inject)

```bash
# report synthetic prefix blocks so overlap scorer can prefer a worker
curl -s http://127.0.0.1:8080/internal/cache/events -H 'content-type: application/json' -d '{
  "type":"stored",
  "worker_id":"mac-tier-a-01",
  "base_model":"qwen3.5:0.8b",
  "tier":"gpu",
  "hashes":[1,2,3,4,5]
}'
```

## Gateway API key (optional)

```bash
export FORGE_GATEWAY_API_KEY=dev-secret
# clients: Authorization: Bearer dev-secret
```

## RunPod fleet provisioner

Defaults to **dry-run** (`FORGE_FLEET_RUNPOD_DRY_RUN=true`). Real create:

```bash
export FORGE_FLEET_RUNPOD_ENABLED=true
export FORGE_FLEET_RUNPOD_DRY_RUN=false
export FORGE_RUNPOD_API_KEY=...
export FORGE_HF_TOKEN=...
export FORGE_FLEET_RUNPOD_GPU_TYPE="NVIDIA GeForce RTX 4090"
export FORGE_FLEET_RUNPOD_VLLM_MODEL="Qwen/Qwen2.5-0.5B-Instruct"
```

Creates a community pod running `vllm/vllm-openai` with the model. Proxy URL:

`https://<pod-id>-8000.proxy.runpod.net`

Register that URL as an OpenAI-compat **provider** (gRPC worker still needs Tailscale for full worker path).

**Balance warning:** keep dry-run until you accept GPU spend. Always `runpodctl pod list --all` and delete leftovers.

## Local fleet spawn

With `FORGE_FLEET_RUNPOD_ENABLED=false` (default), scale-up starts `bin/forge-worker` processes on free ports.

## Observability

| UI | URL |
|----|-----|
| Prometheus (host scrape) | http://127.0.0.1:9092 |
| Jaeger | http://127.0.0.1:16686 |
| Metrics raw | http://127.0.0.1:9090/metrics |

Enable traces:

```bash
FORGE_OTLP_ENDPOINT=localhost:4317 FORGE_TRACE_SAMPLE_RATIO=1.0 ...
```
