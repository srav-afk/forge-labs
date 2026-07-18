# Forge

**Forge is a self-hostable inference control plane** — the layer that sits *above* model servers and turns a messy collection of GPUs, laptops, and vendor APIs into a single OpenAI-compatible system.

Most stacks stop at “run vLLM” or “run Ollama.” That works until you have more than one place to send a request: a free machine at home, a rented GPU, a hosted API for hard prompts, and applications that should not care which backend answered. Forge is built for that problem. Clients speak one protocol to one gateway; Forge owns discovery, placement, capacity, cost preference, failover, and observability across whatever engines you already run.

It is **not** another serving engine. It does not reimplement continuous batching or PagedAttention. Ollama, vLLM, and OpenAI-compatible providers remain the systems that generate tokens. Forge is the operating layer around them: registry, routing snapshot, scheduler, model catalog, admission control, reliability, hybrid providers, fleet hooks, and metrics — the decisions that matter once inference is a *fleet*, not a single process.

## Why it exists

Inference infrastructure splits into two planes whether you name them or not:

- **Data plane** — tokenize, prefill, decode, stream tokens (vLLM, Ollama, vendor APIs).
- **Control plane** — who is alive, who can serve this model, who is cheapest or least loaded, what happens when a node dies, how you refuse work before you melt the GPU.

Forge is the control plane. One base URL for every tool and service you already use (SDKs, IDEs, agents, batch jobs). Swap or add backends without rewriting clients. Prefer free local capacity when it can serve the model; fall through to paid GPU or SaaS when it cannot. Enforce backpressure with clear 429s instead of silent queue collapse. See, in one place, which worker handled which model.

## What you get

| Surface | Behavior |
|---------|----------|
| **OpenAI-compatible gateway** | Chat and completions, streaming SSE and non-streaming JSON, standard-shaped errors |
| **Worker registry & heartbeats** | Durable identity in Postgres; liveness in Redis; dead nodes leave the live set |
| **Async routing snapshot** | Hot path reads memory, not the database — control state propagates asynchronously |
| **Filter → score scheduler** | Health, model/capability, admission, load, latency, cost, session/prefix affinity, policy and cache-overlap signals |
| **Model catalog** | Named models map to assignments; unknown names fail cleanly; multi-model fleets stay intentional |
| **Admission control** | Per-worker inflight limits; fleet saturation returns 429 with retry semantics |
| **Reliability** | Retry budget, circuit breakers, failover before first token across ranked candidates |
| **Hybrid backends** | First-class local workers (Ollama, vLLM) plus virtual workers for OpenAI-compatible providers (hosted APIs, remote vLLM proxies) |
| **Fleet hooks** | Desired-capacity loop with local process spawn and optional cloud GPU provisioner (opt-in; safe defaults) |
| **Cache registry** | Prefix/block reporting and longest-prefix overlap scoring when workers publish cache state |
| **Observability** | Prometheus metrics end-to-end; optional OpenTelemetry traces |

## Quick start

```bash
./scripts/lab-up.sh

curl -s http://127.0.0.1:8080/v1/models
curl -s http://127.0.0.1:8080/v1/chat/completions \
  -H 'content-type: application/json' \
  -d '{"model":"qwen3.5:0.8b","messages":[{"role":"user","content":"hi"}],"max_tokens":32}'
```

The lab stack uses `development.tier-a.env` and `docker-compose.tier-a.yml` (Postgres on `25432`, Redis on `26379`) so it does not collide with other local databases. Optional secrets live in gitignored `development.secrets.env` (provider and cloud API keys).

```bash
FORGE_ENV_FILE=development.tier-a.env go run ./cmd/forge-controlplane
FORGE_ENV_FILE=development.tier-a.env go run ./cmd/forge-worker

FORGE_ENV_FILE=development.tier-a.env go run ./cmd/forge-catalog models
```

Point any OpenAI client at `http://127.0.0.1:8080/v1`. Use different model names to hit local engines versus registered remote providers — same gateway, different placement.

Operational detail: [`docs/ops-lab.md`](docs/ops-lab.md). Remote GPU notes: [`deploy/runpod/README.md`](deploy/runpod/README.md).

## How a request flows

```text
Client (OpenAI SDK, IDE, agent, curl)
        │
        ▼
forge-controlplane  (:8080 HTTP/SSE, :8081 registry gRPC)
        │  resolve model → filter candidates → score → admit
        ├──────────────────┬──────────────────────────┐
        ▼                  ▼                          ▼
 forge-worker          forge-worker              provider://…
 (Ollama / vLLM)       (another host / GPU)      (hosted or proxy OpenAI API)
```

Applications never hardcode “this is the Mac” or “this is the L4.” They call a model name. Forge selects among live, capable, non-saturated workers using the policy you configured.

## Binaries

| Binary | Role |
|--------|------|
| `forge-controlplane` | Gateway, registry, routing, catalog, planner loop, fleet, providers, cache registry |
| `forge-worker` | Runtime adapter, heartbeats, `WorkerService` gRPC generate stream |
| `forge-catalog` | Catalog, provider, and fleet-policy helpers for day-to-day ops |

```bash
task build    # bin/forge-controlplane, bin/forge-worker, bin/forge-catalog
task lab:up   # compose + migrate + start lab processes
task lab:down
```

## Design influences

Ideas and vocabulary from systems that already treat inference as infrastructure: NVIDIA Dynamo, llm-d, LMCache, Fireworks, Baseten, SimpliSmart, vLLM, SGLang. Forge borrows the control/data split and the OpenAI edge contract; it stays deliberately small and self-hostable.

## License

Apache-2.0.
