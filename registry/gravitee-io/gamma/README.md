---
title: "Gamma"
description: "Gravitee Gamma with Access Management and MongoDB backend"
tags: [ai, messaging, observability, security]
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
gck create --from gravitee-io/gamma
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

Place your Gravitee license key at `$HOME/opt/gravitee/license.key` and gck
mounts it into the cluster; without it, everything that does not need a
license runs as usual. Licensed features (flags marked as such) refuse to
start without the file. To use another path, set it at creation time:

```bash
gck create --from gravitee-io/gamma --set licenseFile=/path/to/license.key
```

## Versions

`imagePrefix` and `imageTag` pick the Gamma and APIM images, `helmChartLocator`
and `helmVersion` the chart. AM is composed from `gravitee-io/am/base`, so its
vars are scoped to that path. Pin both products to a release:

```bash
gck create --from gravitee-io/gamma \
  --set gravitee-io.gamma.imageTag=4.12.20 --set gravitee-io.gamma.helmVersion=4.12.20 \
  --set gravitee-io.am.base.imageTag=4.12.7 --set gravitee-io.am.base.helmVersion=4.12.7
```

To install a chart from somewhere other than helm.gravitee.io, such as an OCI
registry, set `helmChartLocator`. Always give an explicit `helmVersion` with
it: an empty one means the highest tag in the registry, whatever pushed it.

```bash
gck create --from gravitee-io/gamma \
  --set gravitee-io.gamma.helmChartLocator=oci://registry.example.com/helm/apim \
  --set gravitee-io.gamma.helmVersion='4.*'
```
