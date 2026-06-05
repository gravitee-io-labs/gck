---
title: "Vault"
description: "Single-node HashiCorp Vault in dev mode"
tags: [security]
---

# Vault

Deploys a single-node HashiCorp Vault server in dev mode (pre-unsealed,
in-memory backend) with host access on port 30820.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from hashicorp/vault/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Configure the Vault CLI and check the server status:

```bash
export VAULT_ADDR=http://localhost:30820
export VAULT_TOKEN=root
vault status
```

Write and read a test secret:

```bash
vault kv put secret/hello foo=bar
vault kv get secret/hello
```

The Vault UI is available at [http://localhost:30820](http://localhost:30820) (token: `root`).

For a guided introduction, see the [Vault Getting Started tutorials](https://developer.hashicorp.com/vault/tutorials/getting-started).

| Parameter   | Value                     |
|-------------|---------------------------|
| URL         | http://localhost:30820     |
| Root token  | root                      |
| Mode        | dev (in-memory)           |
