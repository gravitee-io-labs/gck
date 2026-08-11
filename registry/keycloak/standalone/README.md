---
title: "Keycloak"
description: "Keycloak identity provider in dev mode"
tags: [security]
---

# Keycloak

Deploys a Keycloak server in development mode into a local Kind cluster
with host access on port 30880. Uses an embedded H2 database with no
persistence -- data is lost on restart.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from keycloak/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Open the Keycloak admin console from your host:

```bash
open http://localhost:30880
```

| Parameter | Value |
|-----------|-------|
| Username  | admin |
| Password  | admin |
