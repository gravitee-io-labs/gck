---
title: "AM"
description: "Gravitee Access Management with your choice of datasource"
tags: [networking, security]
---

# AM

Deploys a full Gravitee Access Management stack (Console UI, Gateway, and
Management API). AM stores its data in PostgreSQL over JDBC unless you pick
another datasource with a `--use-*` flag.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from gravitee-io/am
```

Pick another datasource with `--use-jdbc-mysql` or `--use-mongodb`; the
Context flags table lists every option and its default. For example, AM on
MongoDB:

```bash
gck create --from gravitee-io/am --use-mongodb
```

### Cleanup

```bash
gck delete
```

## Quick Start

Sign in to the Console at [http://localhost:30090](http://localhost:30090)
with the default admin account (`admin` / `adminadmin`).

`gck create` ends by printing how to reach the datasource you picked. With
the default PostgreSQL:

```bash
PGPASSWORD=postgres psql -h localhost -p 30432 -U postgres -d am
```

To configure your first identity provider, follow the Gravitee
[AM quick start guide](https://documentation.gravitee.io/am/getting-started/quickstart-guide).

## License

Place your Gravitee license key at `$HOME/opt/gravitee/license.key` and gck
mounts it into the cluster; without it, everything that does not need a
license runs as usual. Licensed features (flags marked as such) refuse to
start without the file. To use another path, set it at creation time:

```bash
gck create --from gravitee-io/am --set licenseFile=/path/to/license.key
```

## Versions

`imagePrefix` and `imageTag` pick the AM images, `helmChartLocator` and
`helmVersion` the chart. They are declared by `gravitee-io/am/base`, so scope
them to that path:

```bash
gck create --from gravitee-io/am \
  --set gravitee-io.am.base.imageTag=4.12.7 --set gravitee-io.am.base.helmVersion=4.12.7
```

To install a chart from somewhere other than helm.gravitee.io, such as an OCI
registry, set `helmChartLocator`. Always give an explicit `helmVersion` with
it: an empty one means the highest tag in the registry, whatever pushed it.

```bash
gck create --from gravitee-io/am \
  --set gravitee-io.am.base.helmChartLocator=oci://registry.example.com/helm/am \
  --set gravitee-io.am.base.helmVersion='4.*'
```
