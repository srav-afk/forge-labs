# Chaos / load labs

Run after every meaningful change. Lab stack must already be up (`./scripts/lab-up.sh`).

```bash
export FORGE_ENV_FILE=development.tier-a.env
export FORGE_GATEWAY_URL=http://127.0.0.1:8080

./scripts/chaos/01_cost_preference.sh   # needs free + paid workers
./scripts/chaos/02_kill_worker.sh       # kills :50051
./scripts/chaos/03_admission_burst.sh   # set admission limit=1 to see 429s
./scripts/chaos/04_provider_failover.sh # needs baseten (or other) provider
```

All scripts are non-destructive except kill/failover (they stop local workers).
