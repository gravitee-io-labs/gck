---
title: "APIM"
description: "Gravitee API Management with your choice of datasource and analytics backend, licensed or not"
tags: [networking, messaging]
---

# APIM

Deploys a full Gravitee API Management stack (Console, Portal, Gateway, and
Management API). APIM stores its data in PostgreSQL over JDBC and its
analytics in Elasticsearch unless you pick another backend with a `--use-*`
flag. When a Gravitee license is present it is mounted automatically, and
licensed features such as the Kafka Gateway are one flag away.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from gravitee-io/apim
```

Each `--use-*` flag picks one implementation in its group: the datasource
(`--use-jdbc-postgres`, `--use-jdbc-mysql`, `--use-jdbc-mssql`, `--use-mongodb`)
and the analytics backend (`--use-elasticsearch`, `--use-opensearch`). The
Context flags table lists every option and its default. For example, APIM on
MongoDB with OpenSearch:

```bash
gck create --from gravitee-io/apim --use-mongodb --use-opensearch
```

To run without analytics at all, pass `--disable-analytics`.

To run the gateway alone, without a database, pass `--use-dbless`. The gateway
is then configured from Kubernetes resources through the Gravitee Kubernetes
Operator, and no console, portal, management API or analytics backend is
deployed:

```bash
gck create --from gravitee-io/apim --use-dbless
```

### Kafka Gateway

The Kafka Gateway is a licensed feature (see [License](#license)).
`--enable-kafka-gateway` deploys a Kafka broker and makes the APIM gateway
its Kafka proxy: clients connect with the Kafka protocol through
`*.kafka.gck.local:9092` over TLS.

```bash
gck create --from gravitee-io/apim --enable-kafka-gateway
```

This uses DNS for host-based Kafka routing (`*.kafka.gck.local`). After
creating the cluster, run the one-time OS setup so these hostnames resolve
on your machine:

```bash
gck setup dns
```

> The setup command requires `sudo` because it writes to system
> directories: `/etc/resolver/` on macOS, and `systemd-resolved`
> configuration on Linux. Once done, day-to-day `gck create` and
> `gck delete` commands run without elevated privileges.

See the [Networking guide](https://gravitee-io-labs.github.io/gck/docs/guides/networking/#local-dns) for details.

### Cleanup

```bash
gck delete
```

## Quick Start

Sign in to the Console at [http://localhost:30080](http://localhost:30080)
with the default admin account (`admin` / `admin`).

`gck create` ends by printing how to reach the datasource you picked. With
the default PostgreSQL:

```bash
PGPASSWORD=postgres psql -h localhost -p 30432 -U postgres -d gravitee
```

To create your first API, follow the Gravitee
[APIM quick start guide](https://documentation.gravitee.io/apim/getting-started/quickstart-guide).

With `--use-dbless`, the gateway is available at
[http://localhost:30082](http://localhost:30082). Define APIs with GKO custom
resources (`ApiV4Definition`, `ApiDefinition`), following the
[GKO documentation](https://documentation.gravitee.io/gko):

```bash
kubectl apply -f my-api.yaml -n gravitee
```

### Connecting a Kafka client

With `--enable-kafka-gateway`, extract the TLS certificate from the running
cluster:

```bash
kubectl get secret kafka-tls -n gravitee -o jsonpath='{.data.tls\.crt}' | base64 -d > kafka-tls.crt
```

Then configure your Kafka client properties:

```properties
security.protocol=SSL
ssl.truststore.type=PEM
ssl.truststore.location=/path/to/kafka-tls.crt
ssl.endpoint.identification.algorithm=
```

The `ssl.endpoint.identification.algorithm` must be set to empty because the
self-signed certificate covers `*.kafka.gck.local` but broker metadata
addresses use two-level subdomains (e.g. `broker-0-acr.kafka.gck.local`)
that don't match the single-level wildcard.

## License

Place your Gravitee license key at `$HOME/opt/gravitee/license.key` and gck
mounts it into the cluster; without it, everything that does not need a
license runs as usual. Licensed features (flags marked as such) refuse to
start without the file. To use another path, set it at creation time:

```bash
gck create --from gravitee-io/apim --set licenseFile=/path/to/license.key
```
