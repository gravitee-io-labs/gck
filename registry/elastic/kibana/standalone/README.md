---
title: "Kibana"
description: "Kibana dashboard for Elasticsearch"
tags: [observability]
---

# Kibana

Deploys Kibana alongside a single-node Elasticsearch cluster into a
local Kind cluster. Kibana UI is accessible on port 30601 and
Elasticsearch on port 30920.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from elastic/kibana/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Open the Kibana UI from your host:

```bash
open http://localhost:30601
```

| Parameter     | Value                   |
|---------------|-------------------------|
| Kibana UI     | http://localhost:30601   |
| Elasticsearch | http://localhost:30920   |
| Security      | disabled                |
