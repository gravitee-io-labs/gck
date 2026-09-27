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

When a user specifies `from: [mongodb]`, gck reads `.default` and resolves it to `mongodb/standalone`. Defaults chain across multiple levels. A directory with its own `gck.yaml` is resolved directly and its `.default`, if any, is ignored -- a context may also hold child contexts in subdirectories.

## Merge semantics

When contexts are composed, each top-level field is merged as follows:

- **`kind`** -- Scalar fields: child wins if set. `nodes`: child replaces the list, but `extraPortMappings` are union-merged by `(containerPort, protocol)`. `containerdConfigPatches`: child replaces entirely.
- **`components`** -- Matched by name. Helm chart/version: child wins. Value files: appended. Values: deep-merged. Manifest files: appended. Manifests: union by resource identity. Secrets/configMaps: matched by name, child replaces; new names appended. Requirements: appended and deduplicated. Unmatched components are appended.
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
- A default may use the template functions, but not other vars: `default: '{{ env "HOME" }}/opt/gravitee/license.key'` renders, and `default: '{{ .base }}/license.key'` fails with an error naming the var. The same holds for `--set` values. Write a literal `{{` as `{{ "{{" }}`.
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
gck create --from gravitee-io/apim --disable-portal --disable-ui
```

The `use-` prefix is reserved for [alternatives](#alternatives).

### Inheritance from abstract parents

Flags defined on an abstract context are inherited by all concrete contexts that compose from it via `from`. A child context can override an inherited flag by providing its own `gck--{name}.yaml` with the same name.

### Flags that compose contexts

A flag that needs other contexts -- a broker, a datastore -- lists them in `from`. While the flag is active, they are composed ahead of the declaring context during resolution, like an [alternative](#alternatives)'s, and the flag's path-scoped `vars` overrides reach them. The flag's own patch is applied afterwards with the other flags, so it has the last word over the declaring context's values:

```yaml
# gck--enable-streaming.yaml
description: "Deploy a Kafka broker and wire the app to it"
from:
  - kafka/standalone
vars:
  kafka:
    standalone:
      imageTag:
        default: "3.9"
components:
  - name: app
    helm:
      values:
        streaming:
          bootstrapServers: kafka:9092
```

Like an alternative's, a flag's `from` is read before templating and must be literal.

### Cumulative application

Multiple flags can be combined. Each flag's patch is merged on top of the resolved context in the order they appear on the command line, using the same [merge rules]({{< ref "/docs/guides/composing-contexts#merge-rules" >}}) as context composition. Flags are applied after alternatives, so a flag always has the last word over the selected implementation.

### Required local files

Secrets and config maps built from local files or env vars fail the deployment when the input is missing (`onMissing: fail`, the default) or skip themselves (`onMissing: ignore`). `gck create` and `gck patch` check every non-ignored input before touching the cluster. Because secrets merge by name, a flag can make an optional input mandatory by redeclaring it:

```yaml
# gck--enable-signing.yaml
description: "Sign responses (needs the signing key)"
components:
  - name: keys
    k8s:
      secrets:
        - name: signing-key
          fromFile: "{{ .signingKeyFile }}"
          onMissing: fail
```

### Requirements and conflicts

A flag that only works with some other flag or alternative lists it in `requires`. `gck create` fails when a required name is not active; a default alternative counts as active:

```yaml
description: "Deploy Kibana alongside Elasticsearch"
requires:
  - use-elasticsearch
```

A flag that cannot work with one lists it in `conflicts`, and `gck create` refuses the combination:

```yaml
description: "Enable bridge architecture: management API serves as bridge, gateway syncs through it"
conflicts:
  - use-dbless
```

### Disabling components

Flags can fully exclude a component from deployment by setting `enabled: false`. When a component is disabled, it is not installed and any `requires` entries referencing it are silently dropped:

```yaml
description: "Disable Kafka Gateway and related components"
components:
  - name: kafka
    enabled: false
  - name: apim
    helm:
      values:
        gateway:
          kafka:
            enabled: false
```

With this flag active, the `kafka` component is skipped entirely and other components that declare `requires: [{component: kafka}]` proceed without waiting for it. Disabling a component the composition does not contain is a no-op.

To remove a component an [alternative](#alternatives) brings in, disable the whole group with [`disables`](#cascading) instead: the member is then never composed, so its host ports and notes go too.

## Alternatives

An alternative is a flag that picks one implementation among mutually exclusive ones, such as the datasource of a product. Alternatives sharing a `group` form a set: exactly one member is applied on every `gck create`, the `default: true` member unless the user selects another.

```yaml
# gck--use-mongodb.yaml
description: "Store APIM data in MongoDB"
group: datasource
from:
  - mongodb/standalone
vars:
  mongodb:
    standalone:
      imageTag:
        default: "7"
components:
  - name: apim
    helm:
      values:
        mongo:
          uri: mongodb://mongodb:27017/apim
```

| Field | Meaning |
|---|---|
| `group` | Required. Name of the set of mutually exclusive members. |
| `default` | Marks the member applied when the user selects none. Exactly one per group. |
| `from` | Contexts composed when this member is selected, ahead of the declaring context's own `from`. Plain flags may declare it too (see [Flags that compose contexts](#flags-that-compose-contexts)). |
| `vars` | Var declarations and path-scoped overrides, merged with the declaring context's. An alternative's default wins over the context's for the same name. |
| `implies` | Plain flags turned on whenever this member is selected. See [Cascading](#cascading). |

The file name must be `gck--use-{member}.yaml`, and only alternatives may use the `use-` prefix.

### Resolution

Alternatives are applied where they are declared, while the context is resolved. Selecting `--use-mongodb` on a context with `from: [gravitee-io/apim/base]` resolves it as if it declared `from: [mongodb/standalone, gravitee-io/apim/base]`, with the member's body merged after the context's own. Members that are not selected are never composed. Groups are applied in group-name order.

An alternative's `from` is read before templating and must be literal. The rest of the file is rendered once the declaring context is resolved, with the composition's effective vars -- the same vars a plain flag file sees, so a member can use `{{ .imagePrefix }}` declared by an abstract base.

A context that composes one with alternatives inherits them: a context with `from: [gravitee-io/am]` offers AM's `--use-*` flags unless it [pins](#pinning) them, as `gravitee-io/gamma` does. A context cannot redeclare an alternative or a group it inherits.

### Cascading

Selecting a member sometimes makes other features meaningless. Three fields express that in the files:

| Field | On | Effect |
|---|---|---|
| `implies` | alternative | Plain flags turned on whenever the member is selected, as if passed on the CLI. They are applied, saved with the cluster, and visible to `hasFlag`. |
| `disables` | plain flag | Alternative groups switched off while the flag is in force: no member of the group is composed. |
| `conflicts` | plain flag | Flags or alternatives the flag cannot be combined with. |

They chain. `--use-dbless` implies `disable-ui` and `disable-analytics`, and `disable-analytics` disables the `analytics` group, so a DB-less gateway composes no Elasticsearch at all:

```yaml
# gck--use-dbless.yaml
description: "No datasource: the gateway runs DB-less, configured from Kubernetes resources through GKO"
group: datasource
from:
  - gravitee-io/gko
implies:
  - disable-ui
  - disable-analytics
```

```yaml
# gck--disable-analytics.yaml
description: "Disable analytics: no Elasticsearch or OpenSearch, no analytics reporters"
disables:
  - analytics
```

A `disables` flag only switches off groups declared by the same context. Selecting a member of a disabled group explicitly (`--use-opensearch --disable-analytics`) is not an error: the disable wins and gck prints a warning.

### Pinning

A context that only works with one member of an inherited group pins it with `use`. Its users can no longer switch that group, and passing another member fails. `use` only reaches composed contexts: naming one of the context's own alternatives is an error, since the group's `default` already says which member applies.

```yaml
from:
  - gravitee-io/am
use:
  - mongodb
```

Members are named with or without the `use-` prefix in `use`. The same field in your own `gck.yaml` selects members the way `--use-*` flags do, without pinning: the groups stay visible, and a `--use-*` flag on the command line overrides the file's member of its group.

### Flags, alternatives or separate contexts

Use **flags** for optional features within a context -- things that can be toggled on or off without changing the fundamental nature of the deployment (e.g., disabling analytics, removing UIs, enabling debug mode).

Use **alternatives** for interchangeable implementations of something the stack always has (e.g., MongoDB vs PostgreSQL as the datasource, Elasticsearch vs OpenSearch for analytics).

Use **separate context directories** for fundamentally different topologies (e.g., GKO as a Gateway API controller vs the APIM chart, standalone vs clustered). A mode that only removes things -- APIM without a database -- is an alternative that implies the flags removing them.

## Registry organization tips

- Use the `org/edition/product/variant` convention for discoverability
- Extract shared config into `abstract: true` base contexts
- Set `.default` files so users can reference products without spelling out the full variant path
- Offer interchangeable backends as alternatives on one product context rather than one directory per backend
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
