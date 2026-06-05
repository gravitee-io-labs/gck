---
title: "APIM OSS - DB-less"
description: "Gravitee APIM gateway in DB-less mode with the Gravitee Kubernetes Operator"
tags: [networking]
---

# APIM DB-less

Deploys the Gravitee API Management gateway in DB-less mode alongside the
Gravitee Kubernetes Operator (GKO). APIs are defined entirely through
Kubernetes custom resources — no database is involved.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from gravitee-io/oss/apim/dbless
```

### Cleanup

```bash
gck delete
```

## Quick Start

The gateway is available at [http://localhost:30082](http://localhost:30082).
Define APIs using GKO custom resources (`ApiV4Definition`, `ApiDefinition`):

```bash
kubectl apply -f my-api.yaml -n gravitee
```

For a guided introduction, see the Gravitee
[APIM quick start guide](https://documentation.gravitee.io/apim/getting-started/quickstart-guide)
and the [GKO documentation](https://documentation.gravitee.io/gko).

## Endpoints

| Service      | URL                   |
|--------------|-----------------------|
| APIM Gateway | http://localhost:30082 |
