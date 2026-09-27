---
product: Gravitee
paths:
  - registry/gravitee-io/
---

# Gravitee product rules

These instructions apply when working on contexts under `registry/gravitee-io/`.

## No edition directories

Each Gravitee product has one context directory, directly under
`gravitee-io/` (`gravitee-io/apim`, `gravitee-io/am`, `gravitee-io/gamma`,
...). There are no `oss/` or `ee/` segments: whether a deployment is
licensed is decided by the license file on the user's machine, not by the
path they compose.

- A feature that needs a license is an `enable-*` context flag on the
  product (e.g. `--enable-kafka-gateway`, `--enable-alert-engine`), never a
  separate context.
- A product that only exists with a license (Gamma, Edge Stack) still lives
  directly under `gravitee-io/`.

## License handling

The license is mounted **when the file exists**, with no flag to pass.
Every Gravitee-platform product base (APIM, AM) declares a `licenseFile`
var, the `license` component, and optional mounts on the gateway and
management API. Products with their own license format (e.g. Edge Stack)
document their own pattern in their product agent file.

```yaml
vars:
  licenseFile:
    default: '{{ env "HOME" }}/opt/gravitee/license.key'
    description: "Gravitee license key file, mounted into the gateway and management API when it exists"
components:
  - name: license
    type: k8s
    namespace: gravitee
    k8s:
      secrets:
        - name: gravitee-license
          fromFile: "{{ .licenseFile }}"
          onMissing: ignore
```

Mount the secret through `extraVolumes` with `optional: true`, so pods
start without a license. Because an optional volume does not hold the pod
back, the chart component must also declare `requires: [{component: license}]`
(ordering only, like `tls-server`): Gravitee reads the license at startup, and
a pod that starts before the secret exists runs unlicensed even once the
secret appears. Do not set the chart's `license.name`: the APIM
and AM charts only use it together with `license.key` (inline base64), so
it does nothing here.

```yaml
components:
  - name: apim
    helm:
      values:
        gateway:
          extraVolumes: |
            - name: graviteeio-license
              secret:
                secretName: gravitee-license
                optional: true
          extraVolumeMounts: |
            - name: graviteeio-license
              mountPath: /opt/graviteeio-gateway/license
              readOnly: true
        api:
          extraVolumes: |
            - name: graviteeio-license
              secret:
                secretName: gravitee-license
                optional: true
          extraVolumeMounts: |
            - name: graviteeio-license
              mountPath: /opt/graviteeio-management-api/license
              readOnly: true
```

AM uses `/opt/graviteeio-am-gateway/license` and
`/opt/graviteeio-am-management-api/license`.

### Licensed features

A flag for a licensed feature redeclares the license secret, same name,
with `onMissing: fail`. Secrets merge by name, so the flag tightens the
base's entry, and gck's pre-flight check stops before creating the cluster
when the file is missing:

```yaml
# gck--enable-kafka-gateway.yaml
components:
  - name: license
    k8s:
      secrets:
        - name: gravitee-license
          fromFile: "{{ .licenseFile }}"
          onMissing: fail
```

Say so in the flag's `description` ("licensed feature, needs the license
file").

### README requirements

Every Gravitee-platform product README must include a **License** section
with the following content (copy-paste verbatim to keep all READMEs
consistent):

```markdown
## License

Place your Gravitee license key at `$HOME/opt/gravitee/license.key` and gck
mounts it into the cluster; without it, everything that does not need a
license runs as usual. Licensed features (flags marked as such) refuse to
start without the file. To use another path, set it at creation time:

\```bash
gck create --from <context-path> --set licenseFile=/path/to/license.key
\```
```
