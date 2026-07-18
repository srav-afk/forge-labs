#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

for f in /tmp/forge-controlplane.pid /tmp/forge-worker-free.pid /tmp/forge-worker-paid.pid /tmp/forge-worker-smol.pid; do
  if [[ -f "$f" ]]; then
    kill "$(cat "$f")" 2>/dev/null || true
    rm -f "$f"
  fi
done

for p in 8080 8081 9090 50051 50052 50053; do
  for pid in $(lsof -nP -tiTCP:$p -sTCP:LISTEN 2>/dev/null || true); do
    if ps -p "$pid" -o command= 2>/dev/null | grep -q forge; then
      kill "$pid" 2>/dev/null || true
    fi
  done
done

PROJECT="${FORGE_COMPOSE_PROJECT:-forge-tier-a}"
COMPOSE_FILE="${FORGE_COMPOSE_FILE:-docker-compose.tier-a.yml}"
if [[ -f "$COMPOSE_FILE" ]]; then
  docker compose -p "$PROJECT" -f "$COMPOSE_FILE" down || true
fi

echo "lab down"
