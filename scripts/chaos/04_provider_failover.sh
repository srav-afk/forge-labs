#!/usr/bin/env bash
set -euo pipefail
GATEWAY="${FORGE_GATEWAY_URL:-http://127.0.0.1:8080}"
LOCAL_MODEL="${FORGE_CHAOS_MODEL:-qwen3.5:0.8b}"
PROVIDER_MODEL="${FORGE_CHAOS_PROVIDER_MODEL:-nvidia/Nemotron-120B-A12B}"

echo "==> chaos: provider still works when local capacity is gone"
echo "  1) local model while workers up"
curl -s --max-time 30 -o /tmp/chaos-local.json -w "local_http=%{http_code}\n" \
  "$GATEWAY/v1/chat/completions" \
  -H 'content-type: application/json' \
  -d "{\"model\":\"$LOCAL_MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}],\"max_tokens\":8}" || true

echo "  2) kill local ollama workers :50051 :50052 (if any)"
for p in 50051 50052; do
  for pid in $(lsof -nP -tiTCP:$p -sTCP:LISTEN 2>/dev/null || true); do
    echo "    kill $pid on :$p"
    kill "$pid" 2>/dev/null || true
  done
done
sleep 8

echo "  3) local model after kill"
curl -s --max-time 15 -o /tmp/chaos-local-down.json -w "local_after=%{http_code}\n" \
  "$GATEWAY/v1/chat/completions" \
  -H 'content-type: application/json' \
  -d "{\"model\":\"$LOCAL_MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}],\"max_tokens\":4}" || true
head -c 200 /tmp/chaos-local-down.json 2>/dev/null; echo

echo "  4) provider model (needs baseten/provider registered + secrets on CP)"
curl -s --max-time 90 -o /tmp/chaos-provider.json -w "provider_http=%{http_code}\n" \
  "$GATEWAY/v1/chat/completions" \
  -H 'content-type: application/json' \
  -d "{\"model\":\"$PROVIDER_MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"say ok\"}],\"max_tokens\":8}" || true
head -c 300 /tmp/chaos-provider.json 2>/dev/null; echo
echo "expect: local_after=503/404-ish capacity; provider_http=200 if provider configured"
