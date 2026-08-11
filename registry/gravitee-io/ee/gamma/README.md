---
title: "Gamma"
description: "Gravitee Gamma with Access Management and MongoDB backend"
tags: [networking, security]
---

# Gamma

Deploys a full Gravitee Gamma stack alongside Access Management, backed
by MongoDB for persistence and Elasticsearch for analytics.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

This context uses DNS for service routing. After creating the cluster, run the
one-time OS setup so `*.gck.local` hostnames resolve on your machine (may require
`sudo`):

```bash
gck setup dns
```

See the [Networking guide](https://gravitee-io-labs.github.io/gck/docs/guides/networking/#local-dns) for details.

## Usage

### Create

```bash
gck create --from gravitee-io/ee/gamma
```

### Cleanup

```bash
gck delete
```

## Quick Start

Sign in to the Gamma Console at [http://gamma-console.gravitee.gck.local](http://gamma-console.gravitee.gck.local)
with the default admin account (`admin` / `admin`).

The APIM Console is available at [http://apim-console.gravitee.gck.local](http://apim-console.gravitee.gck.local)
with the same credentials, and the AM Console at
[http://am-console.gravitee.gck.local](http://am-console.gravitee.gck.local) (`admin` / `adminadmin`).

To connect Gamma to Access Management:

1. Open the AM Console at [http://am-console.gravitee.gck.local](http://am-console.gravitee.gck.local) and create a service account token.
2. Head to the platform module in the Gamma Console at [http://gamma-console.gravitee.gck.local](http://gamma-console.gravitee.gck.local).
3. Use `http://am-api.gravitee.gck.local` as the AM URL and paste the token.

To get started with Gravitee API Management, follow the
[APIM quick start guide](https://documentation.gravitee.io/apim/getting-started/quickstart-guide).

## License

This is an Enterprise Edition (EE) context. Place your Gravitee license
key at `$HOME/opt/gravitee/license.key` and gck will automatically mount
it into the cluster. If the file is missing, the license component is
silently skipped (`onMissing: ignore`).

To use a different path, override it in your `gck.yaml`:

```yaml
components:
  - name: license
    k8s:
      secrets:
        - name: gravitee-license
          fromFile: '/custom/path/to/license.key'
```
