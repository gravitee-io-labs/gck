---
title: "FastMCP Test Server"
description: "Deterministic MCP server over Streamable HTTP for testing MCP proxies and gateways"
tags: [ai]
---

# FastMCP Test Server

Deploys a small [FastMCP](https://gofastmcp.com) server into a local Kind
cluster, built for testing anything that sits in front of an MCP server —
proxies, gateways, catalogs. It speaks Streamable HTTP and exposes tools,
resources, and prompts, with deterministic behavior: the same call always
returns the same result.

- Endpoint: `http://localhost:30800/mcp` from the host,
  `http://fastmcp:8000/mcp` from inside the cluster.
- Tools: `echo`, `add`, `multiply`, `to_upper`, `concat`, `word_count`,
  `reverse` — trim the roster with the `tools` variable.
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
gck create --from fastmcp/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Run an MCP handshake:

```bash
curl -s http://localhost:30800/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"quickstart","version":"1.0"}}}' -i
```

The response carries an `mcp-session-id` header; pass it back on subsequent
calls (`tools/list`, `tools/call`, `resources/list`, `prompts/list`).

To require authentication, deploy with a token — unauthenticated requests
are then rejected with `401`:

```bash
gck create --from fastmcp/standalone --set authToken=my-secret
```

The server also serves plain HTTP endpoints next to `/mcp`:

- `GET /health` — liveness, returns the instance name.
- `POST /admin/tools/hidden_tool/enable` (and `/disable`) — reveals a
  dormant `hidden_tool` at runtime, so a test can observe a new upstream
  tool appearing without redeploying anything.

Testing something that fronts several MCP servers at once? Compose this
context with another server product:

```bash
gck create --from fastmcp/standalone --from ibm/fast-time-server
```
