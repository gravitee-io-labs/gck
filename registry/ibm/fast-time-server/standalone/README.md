---
title: "Fast Time Server"
description: "Tiny MCP time server over Streamable HTTP with tools, resources and prompts"
tags: [ai]
---

# Fast Time Server

Deploys IBM's [fast-time-server](https://github.com/IBM/mcp-context-forge)
— a tiny, single-binary MCP server for time and timezone operations — into
a local Kind cluster. A handy counterpart when testing MCP proxies,
gateways, or catalogs against a genuinely foreign server implementation: it
speaks Streamable HTTP and exposes tools, resources, and prompts from a
7 MB image that starts instantly.

- Endpoint: `http://localhost:30801` from the host,
  `http://fast-time-server:8080` from inside the cluster — MCP is served
  at the root path.
- Tools: `get_system_time`, `convert_time` (time-dependent by nature —
  assert on shapes, not values).
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
gck create --from ibm/fast-time-server
```

### Cleanup

```bash
gck delete
```

## Quick Start

Run an MCP handshake:

```bash
curl -s http://localhost:30801/ \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"quickstart","version":"1.0"}}}' -i
```

The response carries an `Mcp-Session-Id` header; pass it back on subsequent
calls (`tools/list`, `tools/call`, `resources/list`, `prompts/list`).

To require authentication, deploy with a token — unauthenticated requests
are then rejected with `401`:

```bash
gck create --from ibm/fast-time-server --set authToken=my-secret
```

A `GET /health` endpoint reports liveness.

Testing something that fronts several MCP servers at once? Compose this
context with another server product:

```bash
gck create --from fastmcp/standalone --from ibm/fast-time-server
```
