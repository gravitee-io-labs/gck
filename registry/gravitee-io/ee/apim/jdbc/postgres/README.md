---
title: "APIM EE - JDBC PostgreSQL"
description: "Gravitee APIM Enterprise Edition with PostgreSQL JDBC backend"
tags: [networking, messaging]
---

# APIM EE JDBC PostgreSQL

Deploys a full Gravitee API Management Enterprise Edition stack (Console,
Portal, Gateway, and Management API) backed by PostgreSQL via JDBC for
persistence and Elasticsearch for analytics. The Kafka Gateway is enabled
by default, allowing the APIM Gateway to act as a Kafka proxy — clients
connect using the Kafka protocol via `*.kafka.gck.local:9092` with TLS.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

This context uses DNS for host-based Kafka routing (`*.kafka.gck.local`).
After creating the cluster, run the one-time OS setup so these hostnames
resolve on your machine:

```bash
gck setup dns
```

> The setup command requires `sudo` because it writes to system
> directories: `/etc/resolver/` on macOS, and `systemd-resolved`
> configuration on Linux. Once done, day-to-day `gck create` and
> `gck delete` commands run without elevated privileges.

See the [Networking guide](https://gravitee-io-labs.github.io/gck/docs/guides/networking/#local-dns) for details.

## Usage

### Create

```bash
gck create --from gravitee-io/ee/apim/jdbc/postgres
```

### Cleanup

```bash
gck delete
```

## Quick Start

Sign in to the APIM Console at [http://localhost:30080](http://localhost:30080)
with the default admin account (`admin` / `admin`).

To create your first API, follow the Gravitee
[APIM quick start guide](https://documentation.gravitee.io/apim/getting-started/quickstart-guide).

### Connecting a Kafka client

Extract the TLS certificate from the running cluster:

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
