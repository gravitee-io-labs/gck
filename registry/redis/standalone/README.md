---
title: "Redis"
description: "Single-node Redis deployment for Kubernetes"
tags: [database]
---

# Redis

Deploys a single-node Redis 7 instance into a local Kind cluster with
host access on port 30379. No authentication is required.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from redis/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Connect from your host:

```bash
redis-cli -h localhost -p 30379
```

No authentication is configured.
