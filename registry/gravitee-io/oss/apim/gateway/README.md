---
title: "Gateway API"
description: "Gravitee implementation of the Kubernetes Gateway API"
tags: [networking]
---

# APIM Gateway API

Deploys the Gravitee Kubernetes Operator (GKO) configured as a Kubernetes
Gateway API controller. Sets up a `GatewayClass` and its parameters so that
`Gateway` and `HTTPRoute` resources are reconciled by GKO.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

This context uses DNS to resolve in-cluster services by hostname. After
creating the cluster, run the one-time OS setup so these hostnames resolve
on your machine (may require `sudo`):

```bash
gck setup dns
```

See the [Networking guide](https://gravitee-io-labs.github.io/gck/docs/guides/networking/#local-dns) for details.

## Usage

### Create

```bash
gck create --from gravitee-io/oss/apim/gateway
```

### Cleanup

```bash
gck delete
```

## Quick Start

Create a `Gateway` and an `HTTPRoute` resource and let GKO provision
Gravitee gateway instances automatically:

```bash
kubectl apply -f my-gateway.yaml -n gravitee
kubectl apply -f my-route.yaml -n gravitee
```

For details on the Gateway API model, see the
[Kubernetes Gateway API documentation](https://gateway-api.sigs.k8s.io/)
and the [GKO documentation](https://documentation.gravitee.io/gko).
