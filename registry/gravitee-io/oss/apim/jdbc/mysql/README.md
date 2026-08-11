---
title: "APIM OSS - JDBC MySQL"
description: "Gravitee APIM with MySQL JDBC backend and Elasticsearch"
tags: [networking]
---

# APIM JDBC MySQL

Deploys a full Gravitee API Management stack (Console, Portal, Gateway, and
Management API) backed by MySQL via JDBC for persistence and Elasticsearch
for analytics.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from gravitee-io/oss/apim/jdbc/mysql
```

### Cleanup

```bash
gck delete
```

## Quick Start

Sign in to the Console at [http://localhost:30080](http://localhost:30080)
with the default admin account (`admin` / `admin`).

To create your first API, follow the Gravitee
[APIM quick start guide](https://documentation.gravitee.io/apim/getting-started/quickstart-guide).
