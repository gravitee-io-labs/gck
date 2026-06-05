---
title: "Consul"
description: "Single-node HashiCorp Consul in dev mode"
tags: [networking]
---

# Consul

Deploys a single-node HashiCorp Consul server into a local Kind cluster
with host access on port 30500 (UI). Useful for service discovery and
key-value storage in development.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from hashicorp/consul/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Open the Consul UI from your host:

```bash
open http://localhost:30500
```

| Parameter | Value                   |
|-----------|-------------------------|
| UI        | http://localhost:30500   |
| Auth      | none                    |
