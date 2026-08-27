---
title: "LLM Inference Simulator"
description: "OpenAI-compatible LLM simulator for testing AI gateways without a GPU or provider account"
tags: [ai]
---

# LLM Inference Simulator

Deploys the [llm-d inference simulator](https://github.com/llm-d/llm-d-inference-sim)
into a local Kind cluster: a vLLM-compatible fake LLM server built by the
Kubernetes inference-gateway community for testing what sits in front of an
LLM — proxies, gateways, routers — with no GPU, no model weights, and no
provider account.

- Endpoint: `http://localhost:30802/v1` from the host,
  `http://inference-sim:8000/v1` from inside the cluster.
- API surface: `/v1/chat/completions`, `/v1/completions`, `/v1/responses`,
  `/v1/embeddings`, `/v1/models`, plus the Anthropic `/v1/messages` shape.
- Streaming: SSE with delta chunks, a final `usage` chunk
  (`stream_options.include_usage`), and the `data: [DONE]` terminator.
- Every response carries a real `usage` block (prompt, completion and total
  tokens) — exactly what token- and cost-based policies read.
- Deterministic by default: `echo` mode mirrors the request text back, and
  the `seed` variable keeps generated content reproducible.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from llm-d/inference-sim
```

### Cleanup

```bash
gck delete
```

## Quick Start

Call the simulator like any OpenAI-compatible provider:

```bash
curl -s http://localhost:30802/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}'
```

In `echo` mode the assistant message mirrors your prompt, so assertions are
trivial. Add `"stream": true` (and
`"stream_options": {"include_usage": true}`) to exercise SSE streaming.

The served model name comes from the `model` variable:

```bash
gck create --from llm-d/inference-sim --set model=claude-sonnet-5
```

Beyond the variables, the simulator has flags for failure injection
(429/500/context-length), time-to-first-token and inter-token latency, LoRA
adapters, and fake vLLM metrics — see the
[configuration reference](https://github.com/llm-d/llm-d-inference-sim/blob/main/docs/configuration.md)
and override the component's `args` from your own `gck.yaml` to use them.

Testing something that fronts several upstreams, or needs a misbehaving
one? Compose this context with MockServer:

```bash
gck create --from llm-d/inference-sim --from mockserver/standalone
```
