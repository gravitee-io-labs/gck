---
title: "Jaeger"
description: "Jaeger all-in-one distributed tracing backend"
tags: [observability]
---

# Jaeger

Deploys a Jaeger all-in-one instance into a local Kind cluster with
host access on port 30686 (UI), 30317 (OTLP gRPC), and 30318 (OTLP HTTP).
Uses in-memory storage for lightweight development — traces are lost on
restart.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from jaeger/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Open the Jaeger UI from your host:

```bash
open http://localhost:30686
```

Send a test trace using the OpenTelemetry endpoint:

```bash
curl -X POST http://localhost:30318/v1/traces \
  -H 'Content-Type: application/json' \
  -d '{"resourceSpans":[]}'
```

Storage is in-memory, so traces are lost when the pod restarts.
