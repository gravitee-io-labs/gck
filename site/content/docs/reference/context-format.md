---
title: "Context Format"
weight: 3
type: docs
---

This page is for context **authors** -- the people who create and maintain registry contexts for others to use. If you're just using contexts, see [Composing Contexts]({{< ref "/docs/guides/composing-contexts" >}}).

## Anatomy of a context

A context lives at `{registry}/{context_path}/` and must contain a `gck.yaml`. At minimum, it declares components to deploy:

```yaml
helm:
  repos:
    - name: bitnami
      url: https://charts.bitnami.com/bitnami

components:
  - name: mongodb
    type: helm
    namespace: default
    helm:
      chart: bitnami/mongodb
      version: "16.4.0"
      valueFiles:
        - values.yaml
```

File paths in `valueFiles` and `manifestFiles` are resolved relative to the context directory.

## Component types

### Helm components

The default type. Installs a Helm chart:

```yaml
components:
  - name: my-app
    type: helm
    namespace: my-ns
    helm:
      chart: myrepo/my-app
      version: "2.0.0"
      valueFiles:
        - values.yaml
      values:
        replicas: 1
```

### Kubernetes manifest components

Deploy plain Kubernetes resources by setting `type: k8s`. You can use inline manifests, file references, or both:

```yaml
components:
  - name: routes
    type: k8s
    namespace: my-ns
    k8s:
      manifestFiles:
        - gateway.yaml
        - httproutes.yaml
      manifests:
        - apiVersion: v1
          kind: Service
          metadata:
            name: my-service
          spec:
            type: ClusterIP
            ports:
              - port: 8080
```

File-based resources are applied first, then inline manifests. Both component types participate in the same dependency graph.

## Composition with `from`

Contexts can compose other contexts. List the parent paths in `from`:

```yaml
from:
  - mongodb/standalone
  - elastic/elasticsearch/standalone

components:
  - name: my-app
    requires:
      - component: mongodb
      - component: elasticsearch
    helm:
      chart: myrepo/my-app
```

Parents are resolved left-to-right, then local overrides are applied on top using the standard [merge rules]({{< ref "/docs/guides/composing-contexts#merge-rules" >}}).

### Cross-registry composition

By default, `from` entries are resolved against the same registry. To compose from a different registry:

```yaml
registry: https://other-registry.example.com
from:
  - org/product/base
```

Relative `file://` paths are resolved relative to the child context's directory.

## Abstract contexts

Mark a context as `abstract: true` when it's a shared base that shouldn't be deployed on its own:

```yaml
abstract: true

helm:
  repos:
    - name: myrepo
      url: https://charts.example.com

components:
  - name: app
    helm:
      chart: myrepo/app
      version: "2.0.0"
```

Attempting to deploy an abstract context directly with `gck create` produces an error. Concrete variants must compose from it via `from`.

## Default variant resolution

Add a `.default` file next to variant directories to set the default:

```bash
echo "standalone" > registry/mongodb/.default
```

When a user specifies `from: [mongodb]`, gck reads `.default` and resolves it to `mongodb/standalone`. Defaults chain across multiple levels.

## Merge semantics

When contexts are composed, each top-level field is merged as follows:

- **`kind`** -- Scalar fields: child wins if set. `nodes`: child replaces the list, but `extraPortMappings` are union-merged by `(containerPort, protocol)`. `containerdConfigPatches`: child replaces entirely.
- **`components`** -- Matched by name. Helm chart/version: child wins. Value files: appended. Values: deep-merged. Manifest files: appended. Manifests: union by resource identity. Secrets/configMaps: appended. Requirements: appended and deduplicated. Unmatched components are appended.
- **`helm.repos`** -- Deduplicated by name; child wins on conflict.
- **`features`** -- Each feature block is replaced as a whole if the child defines it; otherwise inherited.
- **`images`** -- `preload`: merge mode (default) unions `refs` and `skip`; replace mode uses only the child's `refs`. `mirrors`: child wins if set.

## Overriding service networking

When your context composes from a child that exposes services via `NodePort`, you might need to switch them to `ClusterIP` because your context handles networking differently.

For **Helm components**, override the values and explicitly clear `nodePort`:

```yaml
components:
  - name: child-service
    helm:
      values:
        service:
          type: ClusterIP
          nodePort: null
```

For **k8s manifest components**, provide a full replacement Service manifest. Manifests are merged by resource identity, so your Service replaces the child's entirely.

## Template variables

Context authors can parameterize their gck.yaml with Go template expressions. Declare variable defaults in a `vars` block and reference them with `{{ .variableName }}`:

```yaml
vars:
  helmVersion: ""
  imageTag: "latest"

components:
  - name: apim
    helm:
      chart: graviteeio/apim
      version: "{{ .helmVersion }}"
      values:
        gateway:
          image:
            tag: "{{ .imageTag }}-debian"
        api:
          image:
            tag: "{{ .imageTag }}-debian"
        ui:
          image:
            tag: "{{ .imageTag }}"

images:
  preload:
    refs:
      - "graviteeio/apim-gateway:{{ .imageTag }}-debian"
      - "graviteeio/apim-management-api:{{ .imageTag }}-debian"
      - "graviteeio/apim-management-ui:{{ .imageTag }}"
```

Users override defaults at deploy time:

```bash
gck create --set imageTag=4.12.0
```

### Rules

- Variable names use **camelCase** (`imageTag`, `helmVersion`, not `image_tag`).
- `vars` is a flat `map[string]string` -- no nesting.
- The `vars` block itself must not contain template expressions.
- Undefined variables with no default cause an error.
- Use the `env` function to reference environment variables: `{{ env "HOME" }}`.
- Use `default` for inline fallbacks: `{{ .myVar | default "fallback" }}`.
- Use `required` for mandatory variables: `{{ .licenseKey | required "licenseKey must be set via --set" }}`.

### Templating in composed contexts

Each context is templated independently during resolution. Parent contexts don't see child vars and vice versa. Only `--set` flows globally across all levels:

```yaml
# Parent: vars: { dbVersion: "15" } uses {{ .dbVersion }}
# Child:  vars: { imageTag: "latest" } uses {{ .imageTag }}
# --set dbVersion=16 --set imageTag=v2 overrides both
```

## Context flags

Context flags let maintainers expose optional toggles without creating separate registry directories for every combination. A flag is defined by placing a `gck--{flag-name}.yaml` patch file alongside the context's `gck.yaml`.

### File format

Flag files use the same schema as `gck.yaml`, with a `description` field that documents what the flag does:

```yaml
description: "Disable the developer portal UI"
components:
  - name: apim
    helm:
      values:
        portal:
          enabled: false
```

### Naming convention

Flag file names must follow the pattern `gck--{flag-name}.yaml` where `flag-name` is lowercase kebab-case: `^[a-z0-9]+(-[a-z0-9]+)*$`. Users activate flags with `--flag-name` on the CLI:

```bash
gck create --from gravitee-io/oss/apim --disable-portal --disable-ui
```

### Inheritance from abstract parents

Flags defined on an abstract context are inherited by all concrete contexts that compose from it via `from`. A child context can override an inherited flag by providing its own `gck--{name}.yaml` with the same name.

### Cumulative application

Multiple flags can be combined. Each flag's patch is merged on top of the resolved context in the order they appear on the command line, using the same [merge rules]({{< ref "/docs/guides/composing-contexts#merge-rules" >}}) as context composition.

### Disabling components

Flags can fully exclude a component from deployment by setting `enabled: false`. When a component is disabled, it is not installed and any `requires` entries referencing it are silently dropped:

```yaml
description: "Disable Elasticsearch and analytics reporters"
components:
  - name: elasticsearch
    enabled: false
  - name: apim
    helm:
      values:
        es:
          enabled: false
```

With this flag active, the `elasticsearch` component is skipped entirely and other components that declare `requires: [{component: elasticsearch}]` proceed without waiting for it.

### When to use flags vs separate contexts

Use **flags** for optional features within a context -- components that can be toggled on or off without changing the fundamental nature of the deployment (e.g., disabling analytics, removing UIs, enabling debug mode).

Use **separate context directories** for fundamentally different backends or topologies (e.g., MongoDB vs PostgreSQL, standalone vs clustered).

## Registry organization tips

- Use the `org/edition/product/variant` convention for discoverability
- Extract shared config into `abstract: true` base contexts
- Set `.default` files so users can reference products without spelling out the full variant path
- Include a `README.md` with front matter (`title`, `description`, `tags`) -- the site generator uses it for the registry browser
- Add `notes.create` declaring the endpoints this context exposes, plus any instructions `gck create` should print after a successful deploy. Notes are merged across every composed context into a single endpoints table, so declare only what this context owns and let variants inherit the rest. Use `when` to gate a row on a [context flag](#context-flags):

```yaml
---
title: APIM
endpoints:
  - name: APIM Portal
    url: http://localhost:30081
    when: '{{ not (hasFlag "disable-portal") }}'
---
Everything has been deployed in the `gravitee` namespace.
```

  See [Contributing -- notes.create]({{< ref "/docs/reference/contributing#notescreate" >}}) for the full format and the checks that keep notes in sync with `gck.yaml`.
