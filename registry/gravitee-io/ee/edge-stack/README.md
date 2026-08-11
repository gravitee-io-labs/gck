---
title: "Edge Stack"
description: "Ambassador Edge Stack API gateway"
tags: [networking]
---

# Edge Stack

Deploys Ambassador Edge Stack, a Kubernetes-native API gateway built on
Envoy Proxy, into a local Kind cluster.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from gravitee-io/ee/edge-stack
```

### Cleanup

```bash
gck delete
```

## Quick Start

Edge Stack is available at [http://localhost:30080](http://localhost:30080)
(HTTP) and [https://localhost:30443](https://localhost:30443) (HTTPS).

To configure routing, create `Mapping` resources in the `ambassador`
namespace. For a guided walkthrough, see the
[Edge Stack getting started tutorial](https://www.getambassador.io/docs/edge-stack/latest/tutorials/getting-started).

## License

This is an Enterprise Edition (EE) context. Place your Edge Stack license
at `$HOME/opt/gravitee/edge-stack/license.jwt` and gck will automatically
mount it into the cluster as the `ambassador-edge-stack` Secret. If the
file is missing, the license component is silently skipped
(`onMissing: ignore`).

To use a different path, override it in your `gck.yaml`:

```yaml
components:
  - name: license
    k8s:
      secrets:
        - name: ambassador-edge-stack
          entries:
            - key: license-key
              fromFile: '/custom/path/to/license.jwt'
```
