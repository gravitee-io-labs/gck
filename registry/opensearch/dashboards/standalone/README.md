---
title: "OpenSearch Dashboards"
description: "OpenSearch Dashboards visualization UI"
tags: [observability]
---

# OpenSearch Dashboards

Deploys OpenSearch Dashboards alongside a single-node OpenSearch cluster
into a local Kind cluster. Dashboards UI is accessible on port 30601 and
OpenSearch on port 30921. Security is disabled for lightweight development.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from opensearch/dashboards/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Open the Dashboards UI from your host:

```bash
open http://localhost:30601
```

Security is disabled on both Dashboards and OpenSearch, so no credentials are needed.
