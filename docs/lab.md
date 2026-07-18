# Lab operations

## Hard rule: one env file for the whole fleet

**Control plane and every worker must use the same `FORGE_ENV_FILE`.**

That file owns:

- `FORGE_DB_URL` (Postgres)
- `FORGE_REDIS_URL` (Redis)
- worker identity / model defaults (for the process you start with that file)

If the gateway and a worker disagree on DB or Redis:

- `/v1/models` may still list catalog rows
- chat returns **`no_capacity`** — heartbeats never land on the control plane’s Redis

```bash
# correct
export FORGE_ENV_FILE=development.tier-a.env
./bin/forge-controlplane
FORGE_ENV_FILE=development.tier-a.env ./bin/forge-worker

# wrong — different stacks
FORGE_ENV_FILE=development.tier-a.env ./bin/forge-controlplane
go run ./cmd/forge-worker   # picks development.env → different ports
```

Workers that share one env file must still use **distinct**:

- `FORGE_WORKER_ID`
- `FORGE_WORKER_ENDPOINT` / `FORGE_WORKER_GRPC_ADDR`
- `FORGE_METRICS_ADDR` (do not force the same metrics port on CP and worker)

## One-shot lab

```bash
./scripts/lab-up.sh      # compose + migrate + build + CP + free worker
./scripts/lab-status.sh
./scripts/lab-down.sh
```

Defaults:

| Piece | Value |
|-------|--------|
| Env | `development.tier-a.env` |
| Compose | `docker-compose.tier-a.yml` project `forge-tier-a` |
| Postgres | `localhost:25432` |
| Redis | `localhost:26379` |
| Gateway | `:8080` |
| Registry gRPC | `:8081` |
| Free worker | `:50051`, metrics `:9091` |

Secrets (API keys): `development.secrets.env` (gitignored). Sourced automatically by `lab-up.sh` when present.

## Prerequisites

- Docker
- Go
- Ollama + a pulled model matching `FORGE_WORKER_MODEL_BASE` (lab default `qwen3.5:0.8b`)

```bash
ollama serve
ollama pull qwen3.5:0.8b
```

## Smoke

```bash
curl -s http://127.0.0.1:8080/v1/models
curl -s http://127.0.0.1:8080/v1/chat/completions \
  -H 'content-type: application/json' \
  -d '{"model":"qwen3.5:0.8b","messages":[{"role":"user","content":"hi"}],"max_tokens":16}'
```

## Gateway auth (optional)

If any keys are configured, `/v1/*` requires auth. `/healthz` and `/internal/cache/events` stay open.

```bash
# single key
export FORGE_GATEWAY_API_KEY=dev-secret

# multiple: rawKey:clientId[:maxConcurrent]
export FORGE_GATEWAY_API_KEYS=sk-alice:alice:8,sk-bob:bob:4
```

```bash
curl -s http://127.0.0.1:8080/v1/models -H "Authorization: Bearer sk-alice"
# response header: X-Request-Id
```

Limits (defaults): `FORGE_GATEWAY_REQUEST_TIMEOUT=5m`, `FORGE_GATEWAY_MAX_BODY_BYTES=1048576`.  
Audit lines are JSON on stdout: `gateway_audit` with request_id, client_id, model, worker_id, latency, tokens, error.

Keys can also live in Postgres table `gateway_api_keys` (hashed); env keys merge with DB on reload.

See also [ops-lab.md](./ops-lab.md) for providers, RunPod, cache inject, and fleet flags.
