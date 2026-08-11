---
title: "OpenSearch"
description: "Single-node OpenSearch cluster"
tags: [search, observability]
---

# OpenSearch

Deploys a single-node OpenSearch cluster into a local Kind cluster with
host access on port 30921. Security is disabled for lightweight development.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from opensearch/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Check the cluster from your host:

```bash
curl http://localhost:30921
curl http://localhost:30921/_cluster/health?pretty
```

Security is disabled, so no credentials are needed.
