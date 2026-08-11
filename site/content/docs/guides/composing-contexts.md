---
title: "Composing Contexts"
weight: 2
type: docs
---

One of gck's key strengths is composition. You can layer contexts together using `from` to build complex stacks from simple, reusable building blocks -- without duplicating configuration. The [Architecture]({{< ref "/docs/reference/architecture#context-composition" >}}) page illustrates how this works visually.

## The basics

The `from` field lists registry paths to compose. Each context is resolved and merged in order, with your local overrides applied last:

```yaml
registry: https://raw.githubusercontent.com/gravitee-io-labs/gck/refs/heads/main/registry
from:
  - elastic/elasticsearch/standalone

kind:
  name: my-cluster

components:
  - name: elasticsearch
    namespace: my-app
```

This says: start from the `elastic/elasticsearch/standalone` context, rename the cluster, and move Elasticsearch into a different namespace.

## Multi-context composition

You can compose multiple independent contexts into a single stack. This is how you assemble real-world environments from reusable pieces:

```yaml
from:
  - mongodb/standalone
  - elastic/elasticsearch/standalone

kind:
  name: my-stack

components:
  - name: mongodb
    namespace: my-app
  - name: elasticsearch
    namespace: my-app
  - name: my-service
    type: helm
    namespace: my-app
    requires:
      - component: mongodb
      - component: elasticsearch
    helm:
      chart: myrepo/my-service
```

Contexts in `from` are merged left-to-right: later entries override earlier ones on conflicts. Your local fields override last.

## Adding an OpenTelemetry collector to a Gravitee stack

The `otel-collector/base` context is a reusable observability layer: it deploys an [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/) into the `observability` namespace that receives OTLP telemetry and prints it to its logs via the `debug` exporter. Compose it onto any APIM or AM context, and turn on the gateway's exporter with the inherited `--enable-otel-collector` flag:

```bash
gck create \
  --from gravitee-io/oss/apim/jdbc/postgres \
  --from otel-collector/base \
  --enable-otel-collector
```

The gateway then exports OTLP traces across namespaces to the collector, which you can follow with `kubectl logs -f deploy/otel-collector -n observability`.

> Compose `otel-collector/base` (the abstract layer), not `otel-collector/standalone`. Because `kind.name` from the last context in `from` wins, appending the standalone variant — which declares its own cluster — would rename the cluster. The abstract base declares no cluster of its own, so the Gravitee context keeps naming the cluster while the collector layers on top. Use `otel-collector/standalone` only when you want a collector on its own dedicated cluster.

The `--enable-otel-collector` flag only wires the gateway's exporter; the collector itself comes from the `otel-collector/base` entry in `from`. Pass both together.

## Storing and viewing traces with Grafana

The `debug` exporter only prints spans, so nothing survives past the collector's log buffer. To keep traces and browse them, compose `grafana/base` instead of `otel-collector/base`. It stacks three layers in one entry — the collector, [Grafana Tempo](https://grafana.com/oss/tempo/) as the trace store, and [Grafana](https://grafana.com/oss/grafana/) with the Tempo datasource already provisioned — and points the collector's trace pipeline at Tempo:

```bash
gck create \
  --from gravitee-io/oss/apim/jdbc/postgres \
  --from grafana/base \
  --enable-otel-collector
```

Grafana comes up on `http://localhost:30300`. The layer contributes its own host port mapping, and because `kind.nodes[].extraPortMappings` are merged as a union, it lands alongside the APIM ports rather than replacing them -- no port-forward needed.

For a hostname instead, add `--enable-route`, which serves Grafana at `http://grafana.gck.local` through the Gateway API and local DNS:

```bash
gck create \
  --from gravitee-io/oss/apim/jdbc/postgres \
  --from grafana/base \
  --enable-otel-collector \
  --enable-route
```

That flag turns on the `gateway` and `dns` features, so a cloud-provider-kind load balancer and the local DNS server come up with the cluster. Run `gck setup dns` once beforehand, and expect a password prompt on macOS for the load balancer's packet tunnel.

> Traces only appear for APIs that have tracing switched on. The `--enable-otel-collector` flag turns it on at the gateway level, but each v4 API also needs `analytics.tracing.enabled` — set it under **Reporter Settings** in the console and redeploy the API. A request that matches no API (a bare `curl` against the gateway returning 404) never produces a span, so an empty Tempo is not by itself a sign that the pipeline is broken.

The whole observability layer lands in an `observability` namespace, leaving the product namespace to the product. That is why the gateway's exporter targets a fully qualified `otel-collector.observability.svc.cluster.local:4317` — a bare service name only resolves inside the caller's own namespace. Within the layer, the collector and Grafana reach Tempo by short name, because all three share a namespace.

Each layer is also available on its own: `grafana/tempo/base` is Tempo with no Grafana, and both have `standalone` variants that bring their own Kind cluster.

> The collector keeps its `debug` exporter alongside the Tempo one, so `kubectl logs -f deploy/otel-collector -n observability` remains the fastest way to check whether spans are arriving at all — useful for telling "the gateway is not exporting" apart from "Tempo is not storing".

## Abstract contexts

When several variants share a common foundation, extract the shared parts into an **abstract** context. Mark it with `abstract: true` -- it can't be deployed on its own, only composed into concrete contexts:

```yaml
# registry/mycompany/myproduct/base/gck.yaml
abstract: true

helm:
  repos:
    - name: myrepo
      url: https://charts.example.com

components:
  - name: app
    namespace: default
    helm:
      chart: myrepo/app
      version: "2.0.0"
      values:
        replicas: 1
```

Concrete variants compose from the abstract base:

```yaml
# registry/mycompany/myproduct/dev/gck.yaml
from:
  - mycompany/myproduct/base

kind:
  name: dev-cluster

components:
  - name: app
    helm:
      values:
        debug: true
```

## Default variant resolution

When a product has multiple variants, the registry can define a **default** so you don't have to spell out the full path. A `.default` file in a directory contains the name of the variant to use:

```
registry/mycompany/myproduct/
├── .default          # contains "dev"
├── dev/
│   └── gck.yaml
└── staging/
    └── gck.yaml
```

With this setup, `from: [mycompany/myproduct]` resolves to `mycompany/myproduct/dev`.

Defaults chain across multiple levels -- gck reads `.default` at each directory until it finds a `gck.yaml`. For example, `from: [elastic]` resolves first to `elastic/elasticsearch` (via `elastic/.default`), then to `elastic/elasticsearch/standalone` (via `elastic/elasticsearch/.default`), where the actual `gck.yaml` lives.

## Config resolution order

When you run `gck create`, gck builds the final configuration by merging multiple layers. Each layer overrides the one before it:

- **User-level base** (`$GCK_HOME/gck.yaml`, defaults to `~/.gck/gck.yaml`) -- Shared settings across all your projects. Use this for things like a custom registry URL, image mirrors, or a default DNS domain. This file is optional.
- **Project-level** (`./gck.yaml` or the path given with `--config`) -- Your project's specific config. This is where you list `from` entries, add components, and set cluster options.
- **Registry contexts** -- Each entry in `from` is fetched and merged left-to-right. Later contexts override earlier ones on conflicts.
- **Embedded defaults** -- gck fills in any remaining gaps with sensible defaults (cluster name, ports, feature flags).

The `--registry` and `--from` CLI flags override the corresponding values from config files, so you can quickly test a different context without editing your `gck.yaml`.

## Local overrides

Beyond composing registry contexts, you can add your own components and Helm repos directly in your project `gck.yaml`. This is useful for supporting services that aren't part of the upstream context:

```yaml
from:
  - mycompany/myproduct/dev

helm:
  repos:
    - name: bitnami
      url: https://charts.bitnami.com/bitnami

components:
  - name: redis
    namespace: my-app
    helm:
      chart: bitnami/redis
      values:
        architecture: standalone
```

If a component name matches one from the context, your values are merged on top. If there's no match, the component is added as a new deployment.

### Using value files

For large overrides, you can use `valueFiles` instead of (or alongside) inline `values`. Paths are resolved relative to the `gck.yaml` directory:

```yaml
components:
  - name: app
    helm:
      valueFiles:
        - values-dev.yaml
      values:
        debug: true
```

Value files from composed contexts are appended in order, with later files taking higher precedence. Inline `values` are merged on top of everything.

### Kubernetes manifest components

You can also deploy plain Kubernetes resources without a Helm chart by setting `type: k8s`:

```yaml
components:
  - name: routes
    type: k8s
    namespace: my-app
    k8s:
      manifestFiles:
        - gateway.yaml
      manifests:
        - apiVersion: v1
          kind: Service
          metadata:
            name: my-service
          spec:
            type: ClusterIP
            ports:
              - port: 8080
                targetPort: 8080
            selector:
              app: my-service
```

For larger manifests, you can use `manifestFiles` to reference external YAML files instead of inlining them. Paths are resolved relative to the `gck.yaml` directory:

```yaml
components:
  - name: routes
    type: k8s
    namespace: my-app
    k8s:
      manifestFiles:
        - gateway.yaml
        - routes.yaml
```

You can combine `manifestFiles` and inline `manifests` in the same component -- both are applied.

### Local secrets and ConfigMaps

A `k8s` component can create Secrets and ConfigMaps from local files or environment variables:

```yaml
components:
  - name: credentials
    type: k8s
    namespace: my-app
    k8s:
      secrets:
        - name: license-key
          fromFile: ./license.key
          onMissing: ignore
        - name: api-credentials
          entries:
            - key: token
              fromFile: ./token.txt
            - key: API_KEY
              fromEnv: MY_API_KEY
      configMaps:
        - name: logging-config
          entries:
            - key: logback.xml
              fromFile: ./logback.xml
```

The `onMissing` field controls behavior when a source file or env var is missing: `fail` (default) aborts the deployment, `ignore` skips the resource with a warning.

## Overriding variables

Registry contexts can declare template variables with defaults using a `vars` block. As a user, you override these at deploy time with `--set` -- no files to edit:

```bash
gck create --from gravitee-io/oss/apim/jdbc/postgres --set imageTag=4.6.0 --set helmVersion=4.6.0
```

This works because the APIM base context declares `vars` with defaults (`imageTag: "latest"`, `helmVersion: ""`), and `--set` values take precedence. Check a context's Variables table on the registry site or run `gck info` to discover which variables it supports.

When a composition chain includes multiple contexts that declare the same variable name (e.g. `imageTag`), a plain `--set` broadcasts to all of them. To target a specific context, use dotted path notation:

```bash
# Override MySQL's imageTag without affecting the product's imageTag
gck create --from gravitee-io/oss/am/jdbc/mysql --set mysql.standalone.imageTag=8.4

# This still broadcasts to every context declaring imageTag
gck create --from gravitee-io/oss/am/jdbc/mysql --set imageTag=4.6.0
```

The system matches the dotted key against known context paths in the composition chain using longest-prefix matching: `mysql.standalone.imageTag` resolves to path `mysql/standalone`, variable `imageTag`.

### Declaring your own variables

You can also declare `vars` in your project-level `gck.yaml` and use template expressions anywhere in the file:

```yaml
vars:
  appVersion: "2.0.0"

from:
  - mycompany/myproduct/dev

components:
  - name: app
    helm:
      version: "{{ .appVersion }}"
      values:
        image:
          tag: "{{ .appVersion }}"
```

Then deploy with:

```bash
gck create --set appVersion=2.1.0
```

### Overriding parent variables in the registry

Context authors can override a parent's variable default by nesting it under the parent's path segments in the `vars` block:

```yaml
# gravitee-io/oss/am/jdbc/mysql/gck.yaml
from:
  - mysql/standalone
  - gravitee-io/oss/am/jdbc/base

vars:
  jdbcDriver:
    default: "mysql"

  # Override mysql/standalone's imageTag (path segments as nested keys)
  mysql:
    standalone:
      imageTag:
        default: "8"
```

Entries with a `default` key are var declarations; entries without are path segments leading to overrides. This eliminates the need to duplicate a parent's manifests just to change a version.

### How --set flows through composition

Each context in the composition chain is rendered with its own effective vars: own defaults, overridden by child path-scoped overrides, overridden by `--set` (broadcast then scoped). Templates use short names (`{{ .imageTag }}`) -- a context's templates can only access its own vars.

### Template functions

Beyond variable substitution, a few built-in functions are available in template expressions:

| Function | Usage | Description |
|----------|-------|-------------|
| `env` | `{{ env "HOME" }}` | Returns the value of an environment variable |
| `default` | `{{ .myVar \| default "fallback" }}` | Returns the fallback when the value is empty |
| `required` | `{{ .myVar \| required "must be set" }}` | Fails with a message when the value is empty |

See [Commands -- Template variables]({{< ref "/docs/reference/commands#template-variables" >}}) for the full reference.

## Dependencies between components

Use `requires` to express inter-component dependencies. gck installs components in dependency order and can wait for readiness:

```yaml
components:
  - name: my-service
    requires:
      - component: mongodb
        conditions:
          ready: true
        selector:
          matchLabels:
            app.kubernetes.io/instance: mongodb
    conditions:
      ready: true
    timeout: 10m
```

## Merge rules

When composing contexts or applying local overrides, gck merges fields following these rules:

| Field | Behavior |
|-------|----------|
| `helm.chart` | Your value wins if non-empty |
| `helm.version` | Your value wins if non-empty |
| `helm.valueFiles` | Your files are appended (higher precedence in Helm) |
| `helm.values` | Deep-merged -- maps recurse, named lists merge by `name`, scalars replace |
| `k8s.manifestFiles` | Your files are appended |
| `k8s.manifests` | Union by resource identity; your version wins on conflict |
| `k8s.secrets` | Your secrets are appended |
| `k8s.configMaps` | Your configMaps are appended |
| `requires` | Your requirements are appended (deduplicated by component name) |
| `conditions` | Your value wins if `ready` is true |
| `selector` | Your value wins if set |
| `timeout` | Your value wins if non-empty |
| `notes.create` | Merged, not overridden -- see [Post-deployment notes](#post-deployment-notes) |

### Post-deployment notes

Most fields above are last-wins. Notes are the exception: every context in the composition contributes, so composing two products cannot silently drop one product's instructions.

Each context's `notes.create` declares the endpoints it exposes in YAML front matter, with free-form prose below. On `gck create`, gck prints the cluster-ready line, then folds every layer into **one endpoints table**, then each layer's prose under its own title, in composition order:

```bash
gck create \
  --from gravitee-io/oss/apim/jdbc/postgres \
  --from grafana/base \
  --enable-otel-collector
```

```
  Cluster "gravitee-apim" is ready.

  Endpoints

    PostgreSQL          localhost:30432
    Elasticsearch       http://localhost:30920  security disabled
    APIM Console        http://localhost:30080
    APIM Portal         http://localhost:30081
    APIM Gateway        http://localhost:30082
    APIM Gateway (TLS)  https://localhost:30084
    APIM API            http://localhost:30083
    Grafana             http://localhost:30300

  PostgreSQL

    Database   gravitee
    User       postgres
    Password   postgres

      PGPASSWORD=postgres psql -h localhost -p 30432 -U postgres -d gravitee

  APIM

    Everything has been deployed in the `gravitee` namespace.

  OpenTelemetry Collector

    The collector receives OTLP telemetry in the `observability` namespace and
    prints it via the debug exporter.

    Watch what it receives:

      kubectl logs -f deploy/otel-collector -n observability

  Grafana

    Browse anonymously, or sign in as admin / admin.
```

Rows merge by `name` and prose blocks by `title`. The first context to declare one fixes its position; a later context that declares the same one replaces it -- which is how a context that changes an inherited service corrects its address rather than contradicting it. Declaring `when: false` on the replacement hides it instead, for a service a later layer stops exposing.

Because inherited rows are kept, a variant usually needs no `notes.create` of its own: `gravitee-io/oss/apim/jdbc/postgres` has none, and still prints all of the above.

The same declarations feed the documentation. Each context's registry page renders an **Endpoints** table built from the resolved composition, with a `From` column naming the context each row came from -- so an endpoint declared on an abstract base like `grafana/base` appears on every concrete variant that composes it, even though abstract contexts get no page of their own.

> Notes are declared rather than derived from the config, so gck's test suite checks the two against each other: a documented `localhost` endpoint must match a mapped host port, and a context that documents localhost endpoints must document every host port it maps. See [Contributing -- notes.create]({{< ref "/docs/reference/contributing#notescreate" >}}) if you are authoring a context.

### Values deep merge

When `helm.values` overlap on the same key, gck picks a strategy based on the value type:

| Value type | Strategy |
|-----------|----------|
| Maps | Recursive deep merge -- each nested key is merged individually |
| Named lists (objects with a `name` key) | Merge by `name` -- same-name entries are overridden, new entries appended |
| Everything else (scalars, plain lists) | Replace -- your value wins |

An empty list (`env: []`) replaces the parent's list entirely -- use this to clear inherited entries.

## Multi-level composition

Composition chains work to arbitrary depth. A grandparent context can be composed by a parent, which is then composed by your project config. gck tracks visited contexts and errors if it detects a cycle.

## Overriding service networking

When composing contexts, you might need to change how a child context exposes services. For Helm components, override the relevant values:

```yaml
components:
  - name: search-engine
    helm:
      values:
        service:
          type: ClusterIP
          nodePort: null    # clear the child's nodePort to avoid Kubernetes rejection
```

For `k8s` manifest components, provide a full replacement Service manifest -- manifests are merged by resource identity, so your Service replaces the child's entirely.
