---
title: "PostgreSQL"
description: "Single-node PostgreSQL deployment for Kubernetes"
tags: [database]
---

# PostgreSQL

Deploys a single-node PostgreSQL 17 instance into a local Kind cluster with
host access on port 30432.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from postgresql/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Connect from your host:

```bash
PGPASSWORD=postgres psql -h localhost -p 30432 -U postgres -d gravitee
```

| Parameter | Value      |
|-----------|------------|
| Host      | localhost  |
| Port      | 30432      |
| Database  | gravitee   |
| User      | postgres   |
| Password  | postgres   |
