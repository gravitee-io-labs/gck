---
title: "Prometheus"
description: "Prometheus metrics collection server"
tags: [observability]
---

# Prometheus

Deploys a Prometheus server into a local Kind cluster with host access
on port 30909. Configured with minimal retention (2h / 256MB) for
lightweight development.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from prometheus/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Open the Prometheus UI from your host:

```bash
open http://localhost:30909
```

| Parameter | Value                   |
|-----------|-------------------------|
| UI        | http://localhost:30909   |
| Retention | 2h / 256MB              |
