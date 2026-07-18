#!/usr/bin/env bash
set -euo pipefail
GATEWAY="${FORGE_GATEWAY_URL:-http://127.0.0.1:8080}"
MODEL="${FORGE_CHAOS_MODEL:-qwen3.5:0.8b}"
PORT="${FORGE_CHAOS_KILL_PORT:-50051}"

echo "==> chaos: kill worker on :$PORT mid-lab"
pid=$(lsof -nP -tiTCP:"$PORT" -sTCP:LISTEN 2>/dev/null | head -1 || true)
if [[ -z "${pid:-}" ]]; then
  echo "no listener on :$PORT — start free worker first"
  exit 1
fi
echo "  killing pid $pid"
kill "$pid" || true
sleep 8
echo "  chat after kill (expect 503/no_capacity if sole assignee):"
curl -s --max-time 15 "$GATEWAY/v1/chat/completions" \
  -H 'content-type: application/json' \
  -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}],\"max_tokens\":4}"
echo
echo "restart worker with same FORGE_ENV_FILE, then re-chat"
