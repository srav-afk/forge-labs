#!/usr/bin/env bash
set -euo pipefail
GATEWAY="${FORGE_GATEWAY_URL:-http://127.0.0.1:8080}"
ENV_FILE="${FORGE_ENV_FILE:-development.tier-a.env}"

echo "FORGE_ENV_FILE=$ENV_FILE"
echo "gateway=$GATEWAY"
echo
for p in 8080 8081 9090 9091 50051 25432 26379; do
  if lsof -nP -iTCP:"$p" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "port :$p LISTEN"
  else
    echo "port :$p free"
  fi
done
echo
if curl -sf "$GATEWAY/healthz" >/dev/null 2>&1; then
  echo "healthz: ok"
  curl -s "$GATEWAY/v1/models" | head -c 400
  echo
else
  echo "healthz: down"
fi
