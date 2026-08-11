---
title: "Grafana Tempo"
description: "Standalone Grafana Tempo trace store with OTLP ingestion"
tags: [observability]
---

# Grafana Tempo

Deploys a standalone [Grafana Tempo](https://grafana.com/oss/tempo/) into a
local Kind cluster, running as a single binary with local filesystem storage.
It accepts OTLP directly on host ports 30317 (gRPC) and 30318 (HTTP) and serves
its query API on host port 30200.

Use this when you want a trace store to point your own tooling at. If you want
traces you can actually look at, use `grafana/standalone` instead -- it adds
Grafana with the Tempo datasource already provisioned, plus an OpenTelemetry
Collector in front.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from grafana/tempo/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Send a trace over OTLP HTTP, noting the trace ID you use:

```bash
curl -X POST http://localhost:30318/v1/traces \
  -H 'Content-Type: application/json' \
  -d '{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"demo"}}]},"scopeSpans":[{"spans":[{"traceId":"5b8efff798038103d269b633813fc60c","spanId":"eee19b7ec3c1b174","name":"GET /demo","kind":2,"startTimeUnixNano":"1700000000000000000","endTimeUnixNano":"1700000000050000000"}]}]}]}'
```

Read it back by ID:

```bash
curl http://localhost:30200/api/traces/5b8efff798038103d269b633813fc60c
```

Traces are retained for 1h.

> Traces are held in an `emptyDir`, so they are lost when the pod restarts and
> compacted blocks are dropped after an hour. That is deliberate for local
> development -- raise `blockRetention` if you need a longer window.

> Ingested traces take a few seconds to become queryable while the current
> block is still being written.
