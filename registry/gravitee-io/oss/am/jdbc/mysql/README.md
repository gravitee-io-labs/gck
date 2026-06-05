---
title: "AM - JDBC MySQL"
description: "Gravitee Access Management with MySQL JDBC backend"
tags: [networking, security]
---

# AM JDBC MySQL

Deploys a full Gravitee Access Management stack (Console UI, Gateway, and
Management API) backed by MySQL via JDBC for persistence.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from gravitee-io/oss/am/jdbc/mysql
```

### Cleanup

```bash
gck delete
```

## Quick Start

Sign in to the Console at [http://localhost:30090](http://localhost:30090)
with the default admin account (`admin` / `adminadmin`).

To configure your first identity provider, follow the Gravitee
[AM quick start guide](https://documentation.gravitee.io/am/getting-started/quickstart-guide).

## Endpoints

| Service        | URL                   |
|----------------|-----------------------|
| AM Console     | http://localhost:30090 |
| AM Gateway     | http://localhost:30092 |
| Management API | http://localhost:30093 |
