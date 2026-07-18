#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

ENV_FILE="${FORGE_ENV_FILE:-development.tier-a.env}"
COMPOSE_FILE="${FORGE_COMPOSE_FILE:-docker-compose.tier-a.yml}"
PROJECT="${FORGE_COMPOSE_PROJECT:-forge-tier-a}"

echo "==> compose up ($PROJECT)"
docker compose -p "$PROJECT" -f "$COMPOSE_FILE" up -d

echo "==> wait postgres"
for i in $(seq 1 30); do
  if docker exec "${PROJECT}-postgres-1" pg_isready -U forge >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

DB_URL="${FORGE_DB_URL:-postgres://forge:forge@127.0.0.1:25432/forge?sslmode=disable}"
echo "==> migrate"
docker run --rm --network host \
  -v "$ROOT/migrations:/migrations:ro" \
  arigaio/atlas:latest migrate apply \
  --dir file:///migrations \
  --url "$DB_URL" || true

echo "==> build binaries"
mkdir -p bin
go build -o bin/forge-controlplane ./cmd/forge-controlplane
go build -o bin/forge-worker ./cmd/forge-worker
go build -o bin/forge-catalog ./cmd/forge-catalog

if [[ -f development.secrets.env ]]; then
  set -a
  # shellcheck disable=SC1091
  . ./development.secrets.env
  set +a
  echo "==> loaded development.secrets.env"
fi

echo "==> start control plane"
export FORGE_ENV_FILE="$ENV_FILE"
if ! curl -sf http://127.0.0.1:8080/healthz >/dev/null 2>&1; then
  nohup ./bin/forge-controlplane > /tmp/forge-controlplane.log 2>&1 &
  echo $! > /tmp/forge-controlplane.pid
  sleep 2
fi

echo "==> start free worker"
if ! lsof -nP -iTCP:50051 -sTCP:LISTEN >/dev/null 2>&1; then
  nohup env FORGE_ENV_FILE="$ENV_FILE" ./bin/forge-worker > /tmp/forge-worker-free.log 2>&1 &
  echo $! > /tmp/forge-worker-free.pid
fi

echo "==> ready"
echo "  models:  curl -s http://127.0.0.1:8080/v1/models"
echo "  chat:    curl -s http://127.0.0.1:8080/v1/chat/completions -H 'content-type: application/json' -d '{\"model\":\"qwen3.5:0.8b\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}],\"max_tokens\":16}'"
echo "  metrics: curl -s http://127.0.0.1:9090/metrics | grep forge_"
echo "  logs:    /tmp/forge-controlplane.log /tmp/forge-worker-free.log"
