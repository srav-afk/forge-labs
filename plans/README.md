# Plans — status board

This is the **canonical engineering board** for Forge. Any session (human or Claude) reads this first. Each plan is a doc in this folder with frontmatter; this table is the index. Build **one plan per day**, in order — never start a plan whose `depends_on` is not `done`.

Status values: `done` · `active` · `next` (ready, unblocked) · `draft` (written, blocked by deps) · `blocked`.

## Control plane track (Go)

| ID | Title | Status | Depends on | Teaches |
|----|-------|--------|-----------|---------|
| cp-00 | Foundations: monorepo, proto, compose, observability baseline | done | — | control/data plane skeleton, go.work+buf, telemetry baseline |
| cp-01 | Worker registry | done | cp-00 | service discovery, durable identity in Postgres |
| cp-02 | Heartbeats & health | done | cp-01 | liveness, ephemeral state in Redis, failure detection |
| cp-03 | Worker runtime adapter (Ollama first) | done | cp-02 | runtime abstraction, streaming gRPC |
| cp-04 | Gateway: OpenAI passthrough (single worker) | done | cp-03 | OpenAI compat, SSE proxy, the data plane |
| cp-05 | Routing snapshot | done | cp-04 | control/data plane split, async state propagation |
| cp-06 | Scheduler 1: least-loaded | done | cp-05 | placement, Filter→Score plugin chain |
| cp-07 | Scheduler 2: health-aware | done | cp-06 | health-aware routing, EWMA latency |
| cp-08 | Capacity & admission control | done | cp-07 | backpressure, admission control, 429s |
| cp-09 | Prefix-hash affinity (cache-aware) | done | cp-08 | cache locality via consistent hashing |
| cp-10 | Reliability: retries, failover, circuit breakers | done | cp-09 | resilience patterns, partial failure |
| cp-11 | Observability deep-dive & tracing | done | cp-10 | RED/USE metrics, distributed tracing |
| cp-12 | Multi-model routing | done | cp-11 | model catalog, (base, adapter) identity, model→worker |
| cp-13 | Heterogeneous fleet: RunPod + vLLM, runtime- & cost-aware | done | cp-12 | capability routing, cost axis, GPU fleet |
| cp-14 | Planner agent (advisory scheduler) | done | cp-13 | async optimizer, policy-writing, SLA/cost objective |
| cp-15 | Fleet management & autoscaling | done | cp-14 | autoscaling, scale-to-zero, lifecycle |
| cp-16 | Hybrid local/provider routing | done | cp-15 | provider fallback (OpenAI/Anthropic/Bedrock/Fireworks) |
| cp-17 | Cache registry (LMCache-style) | done | cp-16, rt-10 | KV cache as shared infrastructure |

## Forge Runtime track (Python/PyTorch)

| ID | Title | Status | Depends on | Teaches |
|----|-------|--------|-----------|---------|
| rt-01 | Minimal transformer inference | next | cp-00 | tokenization, forward pass, greedy/sampling decode |

> **Control-plane ops status (2026-07-18):** cp-00–cp-17 code is on `main`. Live-proven: Ollama multi-worker, catalog, admission, Baseten provider, cache event ingest, catalog CLI, RunPod API client (create gated by dry-run). **Not** auto-spending GPUs by default. **Next product track:** rt-01.
| rt-02 | KV cache fundamentals | draft | rt-01 | KV cache from scratch, prefill vs decode |
| rt-03 | Batching | draft | rt-02 | static/dynamic/continuous batching, throughput vs latency |
| rt-04 | Prefix caching | draft | rt-03 | prefix hashing, lookup, reuse, hit-rate |
| rt-05 | Runtime scheduling | draft | rt-04 | request queues, batch construction, fairness |
| rt-06 | Memory management | draft | rt-05 | KV eviction, LRU, memory accounting |
| rt-07 | Paged KV cache | draft | rt-06 | PagedAttention (educational), fragmentation |
| rt-08 | Speculative decoding | draft | rt-07 | draft+verifier, acceptance/rejection |
| rt-09 | Advanced research topics | draft | rt-08 | disaggregation, distributed KV, MoE routing |
| rt-10 | Worker protocol adapter | draft | rt-05, cp-03 | expose Forge Runtime to the control plane via worker/v1 |

## Plan doc template

Every plan doc follows this shape:

```markdown
---
id: cp-00
title: <title>
status: next | draft | active | done | blocked
depends_on: [<ids>]
next: [<ids>]
teaches: <one line>
---

## Goal
One sentence: what works at the end of this plan.

## Concept
The distributed-systems / inference idea this plan teaches, in plain terms.

## What you build
Concrete components, files, and contracts. Include a worked example
(a sample request, a sample proto message, a sample Redis key, a sample query).

## Done when
Measurable acceptance criteria — a metric visible in Grafana, or a failure
you can inject and survive. No hand-waving.

## How the big platforms do it
The "at scale" contrast: how Dynamo / llm-d / LMCache / Fireworks / Baseten /
SimpliSmart solve this, with source links. Where Forge deliberately differs and why.

## Links
depends_on / next, and any docs/ references.
```

## Working agreement (summary)

- One plan a day; no rushing. The board is canonical; Linear mirrors it (one issue per plan).
- No code comments. Newest stable versions (websearch to confirm). No Bazel. No AI slop.
- Never commit — the user commits manually.
- See `CLAUDE.md` for full conventions and architecture decisions.
