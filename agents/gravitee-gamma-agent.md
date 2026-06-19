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
- **Edge Reactor** — Gamma enables the Gateway Edge Reactor (`gateway.edge.enabled: true`, port 18093) for Edge API deployments and Edge Daemon agent connectivity. The Gamma AIM module provides Edge Management UI for monitoring the edge fleet. Because the APIM chart only adds a ClusterIP-style port for edge (no `nodePort` support), a supplementary NodePort Service (`edge-gateway`) exposes port 18093 on NodePort 30086.
- **Port range** — Uses APIM NodePorts 30080--30086. Port 30085 is the Gamma UI, port 30086 is the Edge Gateway reactor.
- **Composition chain** — Composes `gravitee-io/oss/am/mongodb` (AM + MongoDB) and `elastic/elasticsearch/standalone` (analytics). Does not compose from any APIM base context.
- **Image naming** — Uses standard APIM images with `-debian` suffix on gateway and management API (`apim-gateway`, `apim-management-api`), plus `gamma-ui` for the Gamma console.
- **`--set` scoping** — Because gamma composes with AM, both contexts declare `imageTag` and `imagePrefix`. A broadcast `--set imageTag=X` applies to both. When the caller wants different tags for gamma and AM (common — APIM and AM release on different cadences), scope the override: `--set gravitee-io.ee.gamma.imageTag=X`.
