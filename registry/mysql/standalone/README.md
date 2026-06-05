---
title: "MySQL"
description: "Single-node MySQL deployment for Kubernetes"
tags: [database]
---

# MySQL

Deploys a single-node MySQL 9 instance into a local Kind cluster with
host access on port 30306.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from mysql/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Connect from your host:

```bash
mysql -h 127.0.0.1 -P 30306 -u root -pmysql gravitee
```

| Parameter | Value      |
|-----------|------------|
| Host      | 127.0.0.1  |
| Port      | 30306      |
| Database  | gravitee   |
| User      | root       |
| Password  | mysql      |
