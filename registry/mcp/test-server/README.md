---
title: "MCP Test Server"
description: "Deterministic MCP servers over Streamable HTTP for testing MCP proxies and gateways"
tags: [ai]
---

# MCP Test Server

Deploys two small MCP (Model Context Protocol) servers into a local Kind
cluster, built for testing anything that sits in front of an MCP server —
proxies, gateways, catalogs. Both speak Streamable HTTP and expose tools,
resources, and prompts, with deterministic behavior: the same call always
returns the same result.

The two instances differ on purpose, so a proxy or catalog under test can
tell them apart:

| Instance | Endpoint | Auth | Tools |
|---|---|---|---|
| `mcp-server-alpha` | `http://localhost:30800/mcp` | none | `echo`, `add`, `multiply`, `to_upper` |
| `mcp-server-beta` | `http://localhost:30801/mcp` | bearer token | `concat`, `word_count`, `reverse` |

From inside the cluster, they are reachable at
`http://mcp-server-alpha:8000/mcp` and `http://mcp-server-beta:8000/mcp`.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from mcp/test-server
```

### Cleanup

```bash
gck delete
```

## Quick Start

Run an MCP handshake against the open instance:

```bash
curl -s http://localhost:30800/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"quickstart","version":"1.0"}}}' -i
```

The response carries an `mcp-session-id` header; pass it back on subsequent
calls (`tools/list`, `tools/call`, `resources/list`, `prompts/list`).

The beta instance requires a bearer token (default `gravitee-e2e`,
configurable via the `authToken` variable) — requests without it are
rejected with `401`:

```bash
curl -s http://localhost:30801/mcp \
  -H 'Authorization: Bearer gravitee-e2e' \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"quickstart","version":"1.0"}}}' -i
```

Each instance also serves plain HTTP endpoints next to `/mcp`:

- `GET /health` — liveness, returns the instance name.
- `POST /admin/tools/hidden_tool/enable` (and `/disable`) — reveals a
  dormant `hidden_tool` at runtime, so a test can observe a new upstream
  tool appearing without redeploying anything.
