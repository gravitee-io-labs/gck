---
product: Gravitee Gamma
paths:
  - registry/gravitee-io/ee/gamma/
---

# Gravitee Gamma product rules

These instructions apply when working on contexts under `registry/gravitee-io/ee/gamma/`.

## Upstream repository

Gamma uses the Gravitee API Management Helm chart (`graviteeio/apim`) with Gamma-specific configuration. The upstream source code lives at
<https://github.com/gravitee-io/gravitee-api-management>.

Refer to this repository when you need to:

- Look up default Helm values or chart structure for the `apim` chart.
- Understand Gamma-specific configuration options (`gamma.enabled`, `gammaUi.enabled`).
- Check available Docker images and their tags.

## Key differences from standalone APIM

- **Requires AM** — Gamma has a hard dependency on Access Management. The `gamma` component declares `requires: [am]` so AM must be fully ready before Gamma deploys.
- **MongoDB only** — Gamma does not support JDBC backends. It always composes from `gravitee-io/oss/am/mongodb` which brings MongoDB.
- **Enterprise only** — Gamma lives under `ee/` and requires a license. There is no OSS variant.
- **Gamma-specific chart values** — The APIM chart is deployed with `gamma.enabled: true`, `gammaUi.enabled: true`, and `gravitee_gamma_enabled=true` on the management API environment.
- **Gamma UI image** — An additional image `{{ .imagePrefix }}/gamma-ui:{{ .imageTag }}` is preloaded and deployed alongside the standard APIM images.
- **Port range** — Uses APIM NodePorts 30080--30085 (same range as APIM since Gamma is deployed via the APIM chart). Port 30085 is allocated to the Gamma UI service.
- **Composition chain** — Composes `gravitee-io/oss/am/mongodb` (AM + MongoDB) and `elastic/elasticsearch/standalone` (analytics). Does not compose from any APIM base context.
- **Image naming** — Uses standard APIM images with `-debian` suffix on gateway and management API (`apim-gateway`, `apim-management-api`), plus `gamma-ui` for the Gamma console.
- **`--set` scoping** — Because gamma composes with AM, both contexts declare `imageTag` and `imagePrefix`. A broadcast `--set imageTag=X` applies to both. When the caller wants different tags for gamma and AM (common — APIM and AM release on different cadences), scope the override: `--set gravitee-io.ee.gamma.imageTag=X`.
