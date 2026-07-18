#!/usr/bin/env bash
set -euo pipefail
GATEWAY="${FORGE_GATEWAY_URL:-http://127.0.0.1:8080}"
MODEL="${FORGE_CHAOS_MODEL:-qwen3.5:0.8b}"
N="${FORGE_CHAOS_BURST:-16}"

echo "==> chaos: admission burst ($N concurrent model=$MODEL)"
echo "    tip: FORGE_ADMISSION_PER_WORKER_LIMIT=1 on control plane to force 429s"
rm -f /tmp/chaos-adm-*.code
for i in $(seq 1 "$N"); do
  (
    code=$(curl -s -o /tmp/chaos-adm-$i.json -w "%{http_code}" --max-time 90 \
      "$GATEWAY/v1/chat/completions" \
      -H 'content-type: application/json' \
      -d "{\"model\":\"$MODEL\",\"messages\":[{\"role\":\"user\",\"content\":\"burst $i write two sentences\"}],\"max_tokens\":40}")
    echo "$code" > /tmp/chaos-adm-$i.code
  ) &
done
wait
echo "status histogram:"
cat /tmp/chaos-adm-*.code 2>/dev/null | sort | uniq -c
echo "expect: mix of 200 and 429 when limit is tight; all 200 when limit is high"
