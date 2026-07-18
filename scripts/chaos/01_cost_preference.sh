#!/usr/bin/env bash
set -euo pipefail
GATEWAY="${FORGE_GATEWAY_URL:-http://127.0.0.1:8080}"
MODEL="${FORGE_CHAOS_MODEL:-qwen3.5:0.8b}"
METRICS="${FORGE_METRICS_URL:-http://127.0.0.1:9090/metrics}"
N="${FORGE_CHAOS_N:-5}"

echo "==> chaos: cost preference ($N chats model=$MODEL)"
before=$(curl -sf "$METRICS" | grep 'forge_scheduler_dispatched_total' || true)
echo "$before" | sed 's/^/  before: /'

for i in $(seq 1 "$N"); do
  code=$(curl -s -o /tmp/chaos-cost-$i.json -w "%{http_code}" --max-time 60 \
    "$GATEWAY/v1/chat/completions" \
    -H 'content-type: application/json' \
    -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"say $i\"}],\"max_tokens\":8}")
  echo "  req $i -> $code"
done

after=$(curl -sf "$METRICS" | grep 'forge_scheduler_dispatched_total' || true)
echo "$after" | sed 's/^/  after:  /'
echo "expect: free/cheaper worker_id counters rose more than paid (if both healthy)"
