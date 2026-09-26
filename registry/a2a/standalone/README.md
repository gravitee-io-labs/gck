---
title: "A2A Test Agent"
description: "Deterministic A2A agent for testing A2A clients, proxies and gateways"
tags: [ai]
---

# A2A Test Agent

Deploys a small agent built on the official
[A2A Python SDK](https://github.com/a2aproject/a2a-python) into a local Kind
cluster, for testing anything that talks to or sits in front of an
[Agent2Agent](https://a2a-protocol.org) agent: clients, orchestrators,
proxies, gateways. It has no LLM behind it. The text of each message decides
what the agent does, so every run gives the same result.

- Agent card: `http://localhost:30803/.well-known/agent-card.json`.
- JSON-RPC at `http://localhost:30803/` and HTTP+JSON at
  `http://localhost:30803/rest` from the host; `http://a2a-agent:8000` from
  inside the cluster.
- Protocol: A2A 1.0, with A2A 0.3 accepted on the same endpoints.
- Capabilities: streaming (SSE) and push notifications.
- Authentication is off by default; set the `authToken` variable to require
  a static bearer token.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from a2a/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Fetch the agent card:

```bash
curl -s http://localhost:30803/.well-known/agent-card.json
```

Send a message:

```bash
curl -s http://localhost:30803/ \
  -H 'Content-Type: application/json' \
  -H 'A2A-Version: 1.0' \
  -d '{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"role":"ROLE_USER","messageId":"1","parts":[{"text":"hello"}]}}}'
```

The reply is a `TASK_STATE_COMPLETED` task with an `echo: hello` artifact.

> The `A2A-Version: 1.0` header selects the protocol version. Without it the
> agent assumes A2A 0.3 and expects 0.3 method names such as
> `message/send`.

Stream the reply over SSE:

```bash
curl -sN http://localhost:30803/ \
  -H 'Content-Type: application/json' \
  -H 'A2A-Version: 1.0' \
  -d '{"jsonrpc":"2.0","id":1,"method":"SendStreamingMessage","params":{"message":{"role":"ROLE_USER","messageId":"1","parts":[{"text":"stream 3"}]}}}'
```

### Message keywords

The first word of the message picks the behaviour:

| Message | Behaviour |
|---|---|
| `message <text>` | Replies with a message and creates no task |
| `stream <n>` | Streams `n` artifact chunks, 200 ms apart, then completes (default 3) |
| `slow <seconds>` | Stays `WORKING` for that long, then completes (default 30). Use it for `GetTask` and `CancelTask` |
| `ask` | Goes `INPUT_REQUIRED`; the next message on the same `taskId` completes the task |
| `fail` | Goes `FAILED` |
| anything else | Completes with an `echo: <text>` artifact |

To get a task back before it finishes, set `"configuration":{"returnImmediately":true}`
in `SendMessage`. To receive push notifications, add a
`"taskPushNotificationConfig":{"url":"...","token":"..."}` to the same
configuration. The agent accepts any URL, including in-cluster ones.

### Authentication

Deploy with a token and every request without `Authorization: Bearer <token>`
gets a `401`. The agent card and `/health` stay public, and the card
declares the bearer scheme:

```bash
gck create --from a2a/standalone --set authToken=my-secret
```

### Behind a gateway

The agent card advertises `publicUrl` as the address of its interfaces.
When clients reach the agent through a gateway, set `publicUrl` to the
gateway's URL so the card points at it. Compose the agent with an APIM
context and point the API's endpoint at
`http://a2a-agent.default.svc.cluster.local:8000`:

```bash
gck create --from gravitee-io/ee/apim/mongodb --from a2a/standalone
```

Testing push notifications? Compose with MockServer and use it as the
webhook receiver:

```bash
gck create --from a2a/standalone --from mockserver/standalone
```
