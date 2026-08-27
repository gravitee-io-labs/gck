---
title: "MockServer"
description: "Scriptable HTTP and LLM mock server for testing gateways against upstreams you fully control"
tags: [ai, sink]
---

# MockServer

Deploys [MockServer](https://www.mock-server.com) into a local Kind
cluster: a scriptable mock for any HTTP service, with first-class LLM
response mocking — provider-correct OpenAI/Anthropic/Bedrock completions,
token-by-token SSE streaming, configurable usage tokens, and failure
simulation (429 quotas, mid-stream truncation, malformed chunks). Use it
when a test needs an upstream that misbehaves in precise, scripted ways.

- Endpoint: `http://localhost:31080` from the host,
  `http://mockserver:1080` from inside the cluster. Expectations and the
  mocked routes share the same port.
- Web dashboard: `http://localhost:31080/mockserver/dashboard` — live view
  of received requests and active expectations.
- Nothing is mocked until you say so: tests create expectations over the
  REST API and reset them between scenarios.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from mockserver/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Mock an OpenAI-compatible chat endpoint with one expectation:

```bash
curl -s -X PUT http://localhost:31080/mockserver/expectation \
  -H 'Content-Type: application/json' \
  -d '{"httpRequest":{"method":"POST","path":"/v1/chat/completions"},"httpLlmResponse":{"provider":"OPENAI","model":"gpt-4o","completion":{"text":"canned answer","usage":{"inputTokens":12,"outputTokens":8}}}}'
```

Then call it like a provider:

```bash
curl -s http://localhost:31080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}'
```

Add `"streaming": true` (and optionally `streamingPhysics` with a seed,
tokens-per-second and jitter) inside `completion` to serve token-by-token
SSE with a final usage chunk and `data: [DONE]`. Plain `httpResponse`
expectations, request verification, and OpenAPI-driven mocks work the same
way — see the [MockServer documentation](https://www.mock-server.com/mock_server/getting_started.html)
and [LLM response mocking](https://www.mock-server.com/mock_server/llm_response_mocking.html).

Reset everything between test scenarios:

```bash
curl -s -X PUT http://localhost:31080/mockserver/reset
```

Testing something that fronts several upstreams? Compose this context with
the deterministic LLM simulator:

```bash
gck create --from llm-d/inference-sim --from mockserver/standalone
```
