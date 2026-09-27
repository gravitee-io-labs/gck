---
product: Gravitee AM
paths:
  - registry/gravitee-io/am/
---

# Gravitee AM product rules

These instructions apply when working on contexts under `registry/gravitee-io/am/`.

## Upstream repository

The Gravitee Access Management source code lives at
<https://github.com/gravitee-io/gravitee-access-management>.

Refer to this repository when you need to:

- Look up default Helm values or chart structure for the `am` component.
- Understand AM gateway, console, or management API configuration options.
- Check available Docker images and their tags.
- Verify feature availability across OSS and Enterprise editions.

## Key differences from APIM

- **No Elasticsearch dependency** — AM does not use Elasticsearch for analytics or reporting. Do not add Elasticsearch as a composed context or dependency.
- **Datasource alternatives** — `gravitee-io/am` offers its datasource as the `datasource` alternative group (`gck--use-jdbc-postgres.yaml` default, `gck--use-jdbc-mysql.yaml`, `gck--use-mongodb.yaml`). Add a backend as a new member of that group, not as a new directory.
- **Three repository types** — AM has `management`, `oauth2`, and `gateway` repository types (APIM has only `management`). When configuring JDBC, all three must be set to `jdbc`, in every JDBC alternative.
- **Different JDBC Helm values** — AM uses `jdbc.driver` (short database identifier like `postgresql` or `mysql`), `jdbc.host`, `jdbc.port`, `jdbc.database`, and `jdbc.drivers` (array of JAR download URLs including R2DBC). APIM uses `jdbc.url` (full JDBC URL) and `jdbc.driver` (single JAR URL). Do not copy APIM's JDBC values structure into AM contexts.
- **Image naming** — AM images use the `graviteeio/am-*` prefix (`am-gateway`, `am-management-api`, `am-management-ui`), not `apim-*`.
- **Helm chart** — `graviteeio/am` (not `graviteeio/apim`).
- **Port range** — AM uses NodePorts 30090--30093 (APIM uses 30080--30084). See the port allocation convention in CONTRIBUTING.md.
- **No portal** — AM has no developer portal component. The `--disable-portal` flag does not apply to AM contexts.
