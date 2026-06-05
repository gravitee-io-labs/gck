---
title: "Elasticsearch"
description: "Single-node Elasticsearch cluster"
tags: [search, observability]
---

# Elasticsearch

Deploys a single-node Elasticsearch cluster into a local Kind cluster with
host access on port 30920. Security is disabled for lightweight development.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from elastic/elasticsearch/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Check the cluster from your host:

```bash
curl http://localhost:30920
curl http://localhost:30920/_cluster/health?pretty
```

| Parameter | Value                    |
|-----------|--------------------------|
| URL       | http://localhost:30920    |
| Security  | disabled                 |
