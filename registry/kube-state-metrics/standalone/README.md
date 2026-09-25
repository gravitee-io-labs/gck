---
title: "kube-state-metrics"
description: "Kubernetes object state (restarts, readiness, resource limits) as Prometheus metrics"
tags: [observability]
---

# kube-state-metrics

Deploys [kube-state-metrics](https://github.com/kubernetes/kube-state-metrics)
into the `observability` namespace of a local Kind cluster. It turns the state
of pods, nodes and workloads into Prometheus metrics: container restarts and
the reason for the last termination (`OOMKilled`), readiness, pod phase, and
the resource requests and limits every container was given.

Pair it with a scrape of the kubelet (`/metrics/resource`,
`/metrics/cadvisor`) for CPU and memory usage.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from kube-state-metrics/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Forward the metrics port to your host:

```bash
kubectl -n observability port-forward svc/kube-state-metrics 8080
```

Then, from another terminal, list container restarts:

```bash
curl -s localhost:8080/metrics | grep kube_pod_container_status_restarts_total
```

## Composing with another stack

To watch the pods of another stack, compose the abstract
`kube-state-metrics/base` layer instead. It declares no cluster of its own,
so the stack you compose it onto keeps naming the cluster:

```bash
gck create \
  --from gravitee-io/oss/apim/jdbc/postgres \
  --from kube-state-metrics/base
```

Point any scraper in the cluster at
`kube-state-metrics.observability.svc.cluster.local:8080`.
