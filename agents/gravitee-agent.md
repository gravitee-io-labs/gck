---
product: Gravitee
paths:
  - registry/gravitee-io/
---

# Gravitee product rules

These instructions apply when working on contexts under `registry/gravitee-io/`.

## OSS vs EE

- Open-source products live under `oss/` (e.g. `gravitee-io/oss/apim/`).
- Enterprise Edition products live under `ee/` (e.g. `gravitee-io/ee/apim/`).
- Never place an EE-only feature in an `oss/` context.

## License handling (EE only)

Every context under `ee/` requires a valid Gravitee license key. EE
contexts are configured to automatically pick up the license from the
conventional path:

```
$HOME/opt/gravitee/license.key
```

> In gck.yaml files, reference this path using the `env` template function:
> `{{ env "HOME" }}/opt/gravitee/license.key`.

If you place your license at this location, there is nothing else to
do -- gck will automatically mount it into the cluster as a Kubernetes
Secret and wire it into the gateway and API components.

If you store your license at a different location, you can override the
path from your own project-level `gck.yaml` without modifying the
registry context (see [Overriding the license
path](#overriding-the-license-path) below).

### gck.yaml requirements

The license must be mounted as a Kubernetes Secret with `onMissing: ignore`
so the context still works (gracefully degraded) when the file is absent.

Every Gravitee-platform EE context `gck.yaml` (APIM, AM, etc.) or its
abstract base must include the following components exactly as shown.
Products with their own license format (e.g. Edge Stack) document their
own pattern in their product agent file.

```yaml
components:
  - name: license
    type: k8s
    namespace: gravitee
    k8s:
      secrets:
        - name: gravitee-license
          fromFile: '{{ env "HOME" }}/opt/gravitee/license.key'
          onMissing: ignore
```

Both the **gateway** and **api** components must mount the license volume:

```yaml
components:
  - name: apim
    helm:
      values:
        license:
          name: gravitee-license
        gateway:
          extraVolumes: |
            - name: graviteeio-license
              secret:
                secretName: gravitee-license
          extraVolumeMounts: |
            - name: graviteeio-license
              mountPath: /opt/graviteeio-gateway/license
              readOnly: true
        api:
          extraVolumes: |
            - name: graviteeio-license
              secret:
                secretName: gravitee-license
          extraVolumeMounts: |
            - name: graviteeio-license
              mountPath: /opt/graviteeio-management-api/license
              readOnly: true
```

### Overriding the license path

If you keep your license at a different location, you can override it
in your own `gck.yaml` without touching the registry. The `license`
component is merged by name, so only the `fromFile` field needs to be
set:

```yaml
# user gck.yaml
components:
  - name: license
    k8s:
      secrets:
        - name: gravitee-license
          fromFile: '/custom/path/to/license.key'
```

### README requirements

Every EE context README must include a **License** section with the
following content (copy-paste verbatim to keep all EE READMEs
consistent):

```markdown
## License

This is an Enterprise Edition (EE) context. Place your Gravitee license
key at `$HOME/opt/gravitee/license.key` and gck will automatically mount
it into the cluster. If the file is missing, the license component is
silently skipped (`onMissing: ignore`).

To use a different path, override it in your `gck.yaml`:

\```yaml
components:
  - name: license
    k8s:
      secrets:
        - name: gravitee-license
          fromFile: '/custom/path/to/license.key'
\```
```
