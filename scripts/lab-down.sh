#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

LOG_DIR="${FORGE_LAB_LOG_DIR:-/tmp}"
PROJECT="${FORGE_COMPOSE_PROJECT:-forge-tier-a}"
COMPOSE_FILE="${FORGE_COMPOSE_FILE:-docker-compose.tier-a.yml}"

echo "==> stop forge binaries"
for f in \
  "$LOG_DIR/forge-controlplane.pid" \
  "$LOG_DIR/forge-worker-free.pid" \
  "$LOG_DIR/forge-worker-paid.pid" \
  "$LOG_DIR/forge-worker-smol.pid"
do
  if [[ -f "$f" ]]; then
    pid=$(cat "$f" 2>/dev/null || true)
    if [[ -n "${pid:-}" ]]; then
      kill "$pid" 2>/dev/null || true
      sleep 0.2
      kill -9 "$pid" 2>/dev/null || true
    fi
    rm -f "$f"
  fi
done

for p in 8080 8081 9090 9091 50051 50052 50053; do
  for pid in $(lsof -nP -tiTCP:"$p" -sTCP:LISTEN 2>/dev/null || true); do
    cmd=$(ps -p "$pid" -o command= 2>/dev/null || true)
    if echo "$cmd" | grep -Eq 'forge-controlplane|forge-worker|bin/forge-'; then
      echo "    kill $pid on :$p"
      kill "$pid" 2>/dev/null || true
    fi
  done
done

if [[ -f "$COMPOSE_FILE" ]]; then
  echo "==> compose down ($PROJECT)"
  docker compose -p "$PROJECT" -f "$COMPOSE_FILE" down || true
fi

OBS_FILE="${FORGE_OBS_COMPOSE:-docker-compose.lab-obs.yml}"
if [[ -f "$OBS_FILE" ]]; then
  echo "==> obs compose down"
  docker compose -p "${PROJECT}-obs" -f "$OBS_FILE" down || true
fi

echo "==> lab down"
