---
title: "OpenTelemetry Collector"
description: "Standalone OpenTelemetry Collector for receiving OTLP traces, metrics, and logs"
tags: [observability]
---

# OpenTelemetry Collector

Deploys a standalone [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/)
into a local Kind cluster. It accepts OTLP over gRPC (host port 30317) and HTTP
(host port 30318) and prints everything it receives to its own logs via the
`debug` exporter — a lightweight sink for inspecting telemetry during development.

To send telemetry from a Gravitee stack (APIM or AM) into a collector instead,
compose the reusable `otel-collector/base` layer and enable the export flag — see
[Composing with a Gravitee stack](#composing-with-a-gravitee-stack) below.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from otel-collector/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Send a test trace to the OTLP HTTP endpoint:

```bash
curl -X POST http://localhost:30318/v1/traces \
  -H 'Content-Type: application/json' \
  -d '{"resourceSpans":[]}'
```

Watch received telemetry in the collector's logs:

```bash
kubectl logs -f deploy/otel-collector
```

| Parameter | Value                  |
|-----------|------------------------|
| OTLP gRPC | localhost:30317        |
| OTLP HTTP | http://localhost:30318 |
| Exporter  | debug (stdout)         |

## Composing with a Gravitee stack

The collector layer is also available as an abstract context, `otel-collector/base`,
that deploys the collector into the `gravitee` namespace without its own cluster.
Compose it alongside any APIM or AM context and pass `--enable-otel-collector` to
point the gateway's OpenTelemetry exporter at it:

```bash
gck create \
  --from gravitee-io/oss/apim/jdbc/postgres \
  --from otel-collector/base \
  --enable-otel-collector
```

The gateway then exports OTLP traces to `http://otel-collector:4317`, and you can
follow them with:

```bash
kubectl logs -f deploy/otel-collector -n gravitee
```
