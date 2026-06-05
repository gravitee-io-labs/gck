---
product: Gravitee Edge Stack
paths:
  - registry/gravitee-io/ee/edge-stack/
---

# Gravitee Edge Stack product rules

These instructions apply when working on contexts under `registry/gravitee-io/ee/edge-stack/`.

## Upstream repository

The Ambassador Edge Stack source code lives at
<https://github.com/gravitee-io/apro>.

Refer to this repository when you need to:

- Look up default Helm values or chart structure for the `edge-stack` component.
- Understand Edge Stack configuration options.
- Check available Docker images and their tags.

## License handling

Edge Stack uses a different license layout than other Gravitee EE
products. Do **not** apply the APIM license template from
`gravitee-agent.md` to edge-stack contexts.

Key differences:

| | APIM | Edge Stack |
|---|---|---|
| **Conventional path** | `$HOME/opt/gravitee/license.key` | `$HOME/opt/gravitee/edge-stack/license.jwt` |
| **Template syntax**   | `{{ env "HOME" }}/opt/gravitee/license.key` | `{{ env "HOME" }}/opt/gravitee/edge-stack/license.jwt` |
| **Secret name** | `gravitee-license` | `ambassador-edge-stack` |
| **Secret format** | `fromFile` (whole file) | `entries` with `key: license-key` |
| **Namespace** | `gravitee` | `ambassador` |

### gck.yaml requirements

The license must be mounted as a Kubernetes Secret with `onMissing: ignore`
so the context still works (gracefully degraded) when the file is absent.

```yaml
components:
  - name: license
    type: k8s
    namespace: ambassador
    k8s:
      secrets:
        - name: ambassador-edge-stack
          entries:
            - key: license-key
              fromFile: '{{ env "HOME" }}/opt/gravitee/edge-stack/license.jwt'
          onMissing: ignore
```

The Helm component must reference the secret without creating its own:

```yaml
helm:
  values:
    licenseKey:
      createSecret: false
```

### Overriding the license path

If you keep your license at a different location, you can override it
in your own `gck.yaml` without touching the registry. The `license`
component is merged by name, so only the `fromFile` field needs to be
set:

```yaml
components:
  - name: license
    k8s:
      secrets:
        - name: ambassador-edge-stack
          entries:
            - key: license-key
              fromFile: '/custom/path/to/license.jwt'
```

### README requirements

Every edge-stack context README must include a **License** section with
the following content:

```markdown
## License

This is an Enterprise Edition (EE) context. Place your Edge Stack license
at `$HOME/opt/gravitee/edge-stack/license.jwt` and gck will automatically
mount it into the cluster as the `ambassador-edge-stack` Secret. If the
file is missing, the license component is silently skipped
(`onMissing: ignore`).

To use a different path, override it in your `gck.yaml`:

\```yaml
components:
  - name: license
    k8s:
      secrets:
        - name: ambassador-edge-stack
          entries:
            - key: license-key
              fromFile: '/custom/path/to/license.jwt'
\```
```

## Helm chart

- **Chart:** `datawire/edge-stack`
- **Repo:** `https://s3.amazonaws.com/datawire-static-files/charts`
- **Image:** `docker.io/datawire/aes`
- **Namespace:** `ambassador`
