# Forge

Forge is a learning-first, self-hostable **inference operating system** — a control plane that sits above inference runtimes (Ollama, MLX, llama.cpp, vLLM, SGLang, and the companion Forge Runtime) and gives them an OpenAI-compatible gateway, a worker registry, a scheduler, a router, a cache registry, a fleet manager, and full observability.

Forge is **not** another model-serving engine. It is the layer above one — the place where placement, routing, reliability, and fleet decisions are made.

## Two tracks

- **Forge (control plane)** — Go. Runs on a Mac with Docker Compose today; Kubernetes + GPUs (RunPod) later.
- **Forge Runtime** — Python/PyTorch. An educational inference engine built from first principles (attention, KV cache, batching, prefix caching, paged attention, speculative decoding), exposed to the control plane as just another runtime.

## Status

Early. The roadmap lives in [`plans/`](plans/README.md) — one plan per milestone, built one a day. Start there.

## Design influences

NVIDIA Dynamo, llm-d, LMCache, Fireworks AI, Baseten, SimpliSmart, vLLM, SGLang. See [`docs/landscape/`](docs/landscape) for what each does and how Forge relates.

## License

Apache-2.0.
