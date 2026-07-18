#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ENV_FILE="${FORGE_ENV_FILE:-development.tier-a.env}"
COMPOSE_FILE="${FORGE_COMPOSE_FILE:-docker-compose.tier-a.yml}"
PROJECT="${FORGE_COMPOSE_PROJECT:-forge-tier-a}"
GATEWAY="${FORGE_GATEWAY_URL:-http://127.0.0.1:8080}"
DB_URL="${FORGE_DB_URL:-postgres://forge:forge@127.0.0.1:25432/forge?sslmode=disable}"
LOG_DIR="${FORGE_LAB_LOG_DIR:-/tmp}"

die() { echo "error: $*" >&2; exit 1; }

[[ -f "$ENV_FILE" ]] || die "env file not found: $ENV_FILE (set FORGE_ENV_FILE)"
[[ -f "$COMPOSE_FILE" ]] || die "compose file not found: $COMPOSE_FILE"

echo "==> HARD RULE: control plane and every worker MUST use the same FORGE_ENV_FILE"
echo "    FORGE_ENV_FILE=$ENV_FILE"
echo "    (same Postgres + Redis; mismatched env = models list but no_capacity)"

if [[ -f development.secrets.env ]]; then
  set -a
  # shellcheck disable=SC1091
  . ./development.secrets.env
  set +a
  echo "==> loaded development.secrets.env (not committed)"
fi

export FORGE_ENV_FILE="$ENV_FILE"
# shellcheck disable=SC1090
set -a; . "./$ENV_FILE"; set +a
DB_URL="${FORGE_DB_URL:-$DB_URL}"

echo "==> compose up ($PROJECT)"
docker compose -p "$PROJECT" -f "$COMPOSE_FILE" up -d

echo "==> wait postgres"
ok=0
for _ in $(seq 1 40); do
  if docker exec "${PROJECT}-postgres-1" pg_isready -U forge >/dev/null 2>&1; then
    ok=1
    break
  fi
  sleep 1
done
[[ "$ok" == "1" ]] || die "postgres not ready (${PROJECT}-postgres-1)"

echo "==> wait redis"
ok=0
for _ in $(seq 1 20); do
  if docker exec "${PROJECT}-redis-1" redis-cli ping 2>/dev/null | grep -q PONG; then
    ok=1
    break
  fi
  sleep 1
done
[[ "$ok" == "1" ]] || die "redis not ready"

echo "==> migrate ($DB_URL)"
docker run --rm --network host \
  -v "$ROOT/migrations:/migrations:ro" \
  arigaio/atlas:latest migrate apply \
  --dir file:///migrations \
  --url "$DB_URL"

echo "==> build binaries"
mkdir -p bin
go build -o bin/forge-controlplane ./cmd/forge-controlplane
go build -o bin/forge-worker ./cmd/forge-worker
go build -o bin/forge-catalog ./cmd/forge-catalog

free_port() {
  local p=$1
  if lsof -nP -iTCP:"$p" -sTCP:LISTEN >/dev/null 2>&1; then
    return 1
  fi
  return 0
}

start_cp() {
  if curl -sf "$GATEWAY/healthz" >/dev/null 2>&1; then
    echo "==> control plane already up ($GATEWAY)"
    return
  fi
  free_port 8080 || die "port 8080 busy (not forge healthz); free it or set FORGE_HTTP_ADDR"
  free_port 8081 || die "port 8081 busy"
  free_port 9090 || echo "warn: :9090 busy; set FORGE_METRICS_ADDR if CP fails to bind"
  echo "==> start control plane (FORGE_ENV_FILE=$ENV_FILE)"
  nohup env FORGE_ENV_FILE="$ENV_FILE" ./bin/forge-controlplane \
    >"$LOG_DIR/forge-controlplane.log" 2>&1 &
  echo $! >"$LOG_DIR/forge-controlplane.pid"
  for _ in $(seq 1 30); do
    if curl -sf "$GATEWAY/healthz" >/dev/null 2>&1; then
      echo "    healthz ok"
      return
    fi
    sleep 0.5
  done
  die "control plane did not become healthy; see $LOG_DIR/forge-controlplane.log"
}

start_worker() {
  if lsof -nP -iTCP:50051 -sTCP:LISTEN >/dev/null 2>&1; then
    echo "==> worker already listening on :50051"
    return
  fi
  echo "==> start free worker (FORGE_ENV_FILE=$ENV_FILE, metrics :9091)"
  nohup env FORGE_ENV_FILE="$ENV_FILE" FORGE_METRICS_ADDR=:9091 ./bin/forge-worker \
    >"$LOG_DIR/forge-worker-free.log" 2>&1 &
  echo $! >"$LOG_DIR/forge-worker-free.pid"
  for _ in $(seq 1 20); do
    if lsof -nP -iTCP:50051 -sTCP:LISTEN >/dev/null 2>&1; then
      echo "    worker grpc :50051 ok"
      return
    fi
    sleep 0.5
  done
  die "worker did not bind :50051; see $LOG_DIR/forge-worker-free.log"
}

start_cp
start_worker

echo "==> wait for /v1/models (heartbeat + catalog)"
models_ok=0
for _ in $(seq 1 30); do
  body=$(curl -sf "$GATEWAY/v1/models" 2>/dev/null || true)
  if echo "$body" | grep -q '"data"'; then
    echo "$body" | head -c 200
    echo
    models_ok=1
    break
  fi
  sleep 1
done
[[ "$models_ok" == "1" ]] || echo "warn: /v1/models empty — is ollama running and model pulled?"

if command -v ollama >/dev/null 2>&1; then
  if ! curl -sf http://127.0.0.1:11434/api/tags >/dev/null 2>&1; then
    echo "warn: ollama not reachable on :11434 — start with: ollama serve"
  fi
else
  echo "warn: ollama CLI not found — local generation needs ollama"
fi

echo
echo "==> lab ready"
echo "    FORGE_ENV_FILE=$ENV_FILE   # use this for EVERY binary"
echo "    gateway:  $GATEWAY"
echo "    models:   curl -s $GATEWAY/v1/models"
echo "    chat:     curl -s $GATEWAY/v1/chat/completions -H 'content-type: application/json' \\"
echo "              -d '{\"model\":\"qwen3.5:0.8b\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}],\"max_tokens\":16}'"
echo "    metrics:  curl -s http://127.0.0.1:9090/metrics | grep forge_"
echo "    logs:     $LOG_DIR/forge-controlplane.log $LOG_DIR/forge-worker-free.log"
echo "    down:     ./scripts/lab-down.sh"
echo "    docs:     docs/lab.md"
