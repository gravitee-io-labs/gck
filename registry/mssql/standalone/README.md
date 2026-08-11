---
title: "MSSQL Server"
description: "Single-node Microsoft SQL Server deployment for Kubernetes"
tags: [database]
---

# MSSQL Server

Deploys a single-node Microsoft SQL Server 2022 instance into a local Kind
cluster with host access on port 31433. An init Job creates a `gravitee`
database on first startup.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from mssql/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Connect from your host using `sqlcmd`:

```bash
sqlcmd -S localhost,31433 -U SA -P 'Password1!' -C
```

| Parameter | Value      |
|-----------|------------|
| Database  | gravitee   |
| User      | SA         |
| Password  | Password1! |
