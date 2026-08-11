---
title: "Grafana"
description: "Grafana with Tempo and an OpenTelemetry Collector as a ready-to-use trace sink"
tags: [observability]
---

# Grafana

Deploys a complete trace sink into a local Kind cluster: an OpenTelemetry
Collector that accepts OTLP on host ports 30317 (gRPC) and 30318 (HTTP),
[Grafana Tempo](https://grafana.com/oss/tempo/) storing the traces it receives,
and [Grafana](https://grafana.com/oss/grafana/) on host port 30300 with the
Tempo datasource already provisioned.

Point any OpenTelemetry-instrumented application at the collector and the traces
become searchable in Grafana within seconds -- no datasource setup, no login.

To send traces from a Gravitee stack running in the same cluster, compose
`grafana/base` onto an APIM or AM context instead -- see
[Composing with a Gravitee stack](#composing-with-a-gravitee-stack) below.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

Grafana is reachable on `http://localhost:30300` with no extra setup. If you
would rather serve it on a hostname, pass `--enable-route` (see
[Serving Grafana on a hostname](#serving-grafana-on-a-hostname)); that turns on
local DNS, which needs a one-time OS setup (may require `sudo`):

```bash
gck setup dns
```

See the [Networking guide](https://gravitee-io-labs.github.io/gck/docs/guides/networking/#local-dns) for details.

## Usage

### Create

```bash
gck create --from grafana/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Open Grafana and go to **Explore** -- the Tempo datasource is selected by
default, so **Search** lists every trace received so far:

```bash
open http://localhost:30300
```

Anonymous access is enabled with the Admin role, so no login is required. The
built-in `admin` / `admin` account still works if you want a named user.

Send a trace to confirm the pipeline end to end:

```bash
curl -X POST http://localhost:30318/v1/traces \
  -H 'Content-Type: application/json' \
  -d '{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"demo"}}]},"scopeSpans":[{"spans":[{"traceId":"5b8efff798038103d269b633813fc60c","spanId":"eee19b7ec3c1b174","name":"GET /demo","kind":2,"startTimeUnixNano":"1700000000000000000","endTimeUnixNano":"1700000000050000000"}]}]}]}'
```

Then search for the `demo` service in Explore. Traces are also printed to the
collector's own logs, which is the quickest way to tell whether something is
arriving at all:

```bash
kubectl logs -f deploy/otel-collector
```

| Parameter | Value                       |
|-----------|-----------------------------|
| Login     | anonymous, or admin / admin |
| Retention | 1h                          |

> Traces are held in an `emptyDir`, so they are lost when the pod restarts and
> compacted blocks are dropped after an hour. That is deliberate for local
> development -- raise `blockRetention` if you need a longer window.

## Serving Grafana on a hostname

`http://localhost:30300` works everywhere and needs nothing set up, which makes
it the right default. For a demo you may prefer a real hostname. Pass
`--enable-route` to serve Grafana through the Kubernetes Gateway API with a
local DNS record instead:

```bash
gck create --from grafana/standalone --enable-route
```

Grafana is then available at `http://grafana.gck.local`, alongside the NodePort.
Override the hostname with `--set hostname=grafana.demo.gck.local`.

The flag turns on gck's `gateway` and `dns` features, which brings up a
cloud-provider-kind load balancer and the local DNS server. On macOS the load
balancer needs a packet tunnel, so `gck create` prompts for your password, and
`gck setup dns` must have been run once beforehand.

## Composing with a Gravitee stack

`grafana/base` is the same layer without a cluster of its own: the collector,
Tempo and Grafana are deployed into an `observability` namespace, leaving the
product namespace to the product. Compose it onto any APIM or AM context and
pass `--enable-otel-collector` to point the gateway's exporter at the collector:

```bash
gck create \
  --from gravitee-io/oss/apim/jdbc/postgres \
  --from grafana/base \
  --enable-otel-collector
```

Grafana comes up on `http://localhost:30300` in that cluster too -- the layer
brings its own host port mapping, so it works alongside the APIM ports without
any port-forward. Add `--enable-route` for `http://grafana.gck.local`.

Call an API through the gateway, then search Tempo for the `gio-apim-gateway`
service in **Explore**.

> The gateway only emits spans for APIs that have tracing switched on. Enabling
> it on the gateway is not enough: for each v4 API, open **Reporter Settings**
> in the console and turn on tracing (`analytics.tracing.enabled`), then
> redeploy the API. Until you do, the pipeline is healthy but carries nothing.
> Requests that match no API — a bare `curl` against the gateway returning 404 —
> never produce a span either.

> Compose `grafana/base`, not `grafana/standalone`. The standalone variant
> declares its own Kind cluster and would rename the cluster the Gravitee
> context created. The abstract base declares no cluster of its own, so it
> layers cleanly on top.
