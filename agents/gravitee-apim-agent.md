---
product: Gravitee APIM
paths:
  - registry/gravitee-io/apim/
---

# Gravitee APIM product rules

These instructions apply when working on contexts under `registry/gravitee-io/apim/`.

## Upstream repository

The Gravitee API Management source code lives at
<https://github.com/gravitee-io/gravitee-api-management>.

Refer to this repository when you need to:

- Look up default Helm values or chart structure for the `apim` component.
- Understand APIM gateway, console, or portal configuration options.
- Check available Docker images and their tags.
- Verify feature availability across OSS and Enterprise editions.

## Backends

`gravitee-io/apim` offers two alternative groups:

- `datasource`: `gck--use-jdbc-postgres.yaml` (default), `gck--use-jdbc-mysql.yaml`, `gck--use-jdbc-mssql.yaml`, `gck--use-mongodb.yaml`, and `gck--use-dbless.yaml` (no database: gateway only, configured through GKO). Every JDBC member sets `management.type: jdbc` and the gateway `gravitee_ratelimit_*` env itself. `use-dbless` implies `disable-ui` and `disable-analytics`; flags that need the management API (`enable-bridge`, `enable-keycloak`, `enable-mailhog`, `enable-distributed-sync`) and `enable-gko` declare `conflicts: [use-dbless]` -- add it to any new flag of that kind.
- `analytics`: `gck--use-elasticsearch.yaml` (default), `gck--use-opensearch.yaml`. `apim/base` does not wire an analytics backend; the members do. `--disable-analytics` turns analytics off whichever member is selected.

Add a backend as a new member of its group, not as a new directory.

## Licensed features

There is one APIM context, licensed or not (see `gravitee-agent.md`). Licensed features are `enable-*` flags on `gravitee-io/apim` that redeclare the license secret with `onMissing: fail`:

- `--enable-kafka-gateway` composes `kafka/standalone` and `gravitee-io/apim/kafka-gateway` (abstract: the Kafka notes and the `kafka-tls` secret) through the flag's `from`, and carries the apim values, `lb`/`dns` features and the kafka ClusterIP override itself. Values that override `apim/base` must stay in the flag body: a composed parent merges before `apim/base` and would lose to it.
- `--enable-alert-engine` deploys Alert Engine.
