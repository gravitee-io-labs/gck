---
title: "Commands"
weight: 2
type: docs
---

All gck commands in one place. Every command respects the global flags listed at the bottom of this page.

## gck create

Create a Kind cluster and deploy the components defined by your context.

```bash
gck create
```

gck resolves the config chain (user-level `$GCK_HOME/gck.yaml` merged with your project `gck.yaml`), fetches contexts from the registry, creates the Kind cluster, and installs components in dependency order. If no registry or context is configured, only the bare cluster is created.

When features like load balancers, Gateway API, or DNS are enabled, gck sets them up automatically after the cluster is ready.

### Flags

| Flag | Description |
|------|-------------|
| `--config <path>` | Project-level config file. Defaults to `./gck.yaml` when present. |
| `--registry <url>` | Registry URL. Overrides the value from config. |
| `--from <path>` | Context path to compose. Repeatable. Overrides the `from` list from config. |
| `--skip-preload` | Skip image preloading even when `images.preload` is configured. |

### Context flags

Contexts can define optional flags that customize the deployment. These are extra `--flag-name` options defined by the context maintainer as `gck--{flag-name}.yaml` patch files.

```bash
gck create --from gravitee-io/oss/apim --disable-portal --disable-ui --disable-es
```

Each flag merges a patch on top of the resolved context before deployment. Flags are cumulative -- you can combine as many as needed. Passing an unknown flag produces an error listing the available flags for that context.

See [Context Format -- Context flags]({{< ref "/docs/reference/context-format#context-flags" >}}) for how to author flag files.

## gck info

Show information about the resolved context without creating a cluster. Displays the component list, available context flags, and enabled features. Use this to discover what flags a context supports before running `gck create`.

```bash
gck info
gck info --from gravitee-io/oss/apim/jdbc/postgres
```

### Example output

```
Context
  Path: gravitee-io/oss/apim/jdbc/postgres

Components
  - postgresql
  - elasticsearch
  - apim

Flags
  --disable-es       Disable Elasticsearch and analytics reporters
  --disable-ui       Disable both Console and Portal UIs
  --disable-portal   Disable the developer portal UI

Features
  lb:      enabled
  gateway: enabled
  dns:     enabled (domain: gck.local, port: 15353)

Usage
  gck create --from gravitee-io/oss/apim/jdbc/postgres
```

## gck build

Build local Docker images, push them to the cluster's preload registry, and restart matching workloads. See the [Developer Loop]({{< ref "/docs/guides/developer-loop" >}}) guide for a walkthrough.

```bash
gck build
gck build gateway console-ui
gck build --create --skip-pre gateway
```

Build entries are defined in the `builds` section of `gck.yaml`. When called without arguments, all entries are built. Pass one or more names to build a subset.

### Flags

| Flag | Description |
|------|-------------|
| `--create` | Create the cluster if it doesn't exist, then build. Context flags are forwarded to the creation step. |
| `--skip-pre` | Skip pre-build commands (`pre`), go straight to `docker build`. |
| `--no-restart` | Build and push but don't restart workloads. |
| `--name <cluster>` | Target a specific cluster. Defaults to `kind.name` from the resolved config. |

### Build logs

All build output (pre-build commands, docker build) is written to `~/.gck/logs/build/build.log`. On failure, gck prints the path so you can inspect the full output.

## gck patch

Upgrade components on a running cluster. There are two modes:

- **With a patch file** -- merges the file into the resolved context and upgrades only the components listed in the file.
- **With `--set` only** -- re-renders the resolved context with new template variable values and upgrades all components.

Both modes can be combined.

```bash
gck patch upgrade.yaml
gck patch upgrade.yaml --dry-run
gck patch --set imageTag=4.11.0 --set helmVersion=4.11.0
gck patch upgrade.yaml --set imageTag=4.11.0
```

### How it works

1. **Resolve context** -- gck loads the config chain and resolves the registry context, exactly as `gck create` does. When `--set` is provided, template variables are overridden during resolution.
2. **Verify cluster** -- gck checks that the target Kind cluster is running.
3. **Load patch file** (when provided) -- the patch file is loaded (same format as `gck.yaml`).
4. **Merge** -- patch components (if any) are merged on top of the resolved context using the standard [merge rules]({{< ref "/docs/guides/composing-contexts#merge-rules" >}}).
5. **Upgrade** -- when a patch file is given, only components named in it are upgraded. When using `--set` alone, all components are upgraded.
6. **Readiness** -- gck waits for upgraded components that have `conditions.ready: true`.

### Patch file format

The patch file uses the same format as `gck.yaml`. Only `components`, `helm.repos`, and `images.preload` are relevant -- other fields are ignored.

#### Upgrade image tags

```yaml
components:
  - name: my-app
    helm:
      values:
        gateway:
          image:
            tag: "2.1.0"
        api:
          image:
            tag: "2.1.0"
```

#### Change a chart version

```yaml
components:
  - name: my-app
    helm:
      version: "2.1.0"
```

#### Add a value file overlay

```yaml
components:
  - name: my-app
    helm:
      valueFiles:
        - ./staging-overrides.yaml
```

Value file paths are resolved relative to the directory containing the patch file.

#### Patch a k8s manifest component

```yaml
components:
  - name: custom-routes
    k8s:
      manifestFiles:
        - ./updated-routes.yaml
```

### Set-only mode

When the registry context uses template variables (see [Composing Contexts -- Template variable overrides]({{< ref "/docs/guides/composing-contexts#template-variable-overrides" >}})), you can upgrade without a patch file by passing new `--set` values:

```bash
# Create the cluster at version 4.10
gck create --from gravitee-io/oss/apim/jdbc/postgres \
  --set imageTag=4.10.0 --set helmVersion=4.10.0

# Upgrade to 4.11 -- no patch file needed
gck patch --set imageTag=4.11.0 --set helmVersion=4.11.0
```

In this mode, the resolved context is re-rendered with the new variable values and all components are upgraded. Use `--dry-run` to preview the changes first.

### Typical workflow

```bash
# Create the cluster with the current version
gck create --set imageTag=4.10.0

# Run tests against the current version
./run-tests.sh

# Upgrade to the new version
gck patch --set imageTag=4.11.0

# Run tests against the new version
./run-tests.sh

# Tear down
gck delete
```

### Image preloading

When the cluster was created with image preloading enabled, the Kind nodes are already configured to pull from the local `gck-preload` registry. Adding `images.preload.refs` to your patch file pre-stages images before upgrading, so pods start faster:

```yaml
images:
  preload:
    refs:
      - myrepo/my-app:2.1.0-rc1
      - myrepo/my-api:2.1.0-rc1

components:
  - name: my-app
    helm:
      values:
        image:
          tag: 2.1.0-rc1
```

If no preload registry is running, gck prints a warning and proceeds normally.

### Dry-run mode

Use `--dry-run` to preview what a patch would change without applying anything:

```bash
gck patch upgrade.yaml --dry-run
```

When `--dry-run` is active:

- **Helm components** run `helm upgrade --dry-run=server` -- the chart is rendered and validated by the API server without creating the release
- **Kubernetes manifest components** run `kubectl apply --dry-run=server` -- objects are validated without persisting
- **Readiness checks are skipped** since no resources are actually deployed
- **Colored diff output** shows exactly what would change: added lines in green, removed lines in red

This is especially useful in CI pipelines:

```bash
gck patch upgrade.yaml --dry-run
gck patch upgrade.yaml
```

### Flags

| Flag | Description |
|------|-------------|
| `--name <cluster>` | Name of the cluster to patch. Defaults to `kind.name` from the resolved config. |
| `--set <key=value>` | Override a template variable. Repeatable. When used without a patch file, all components are upgraded. |
| `--dry-run` | Preview changes without applying. Uses server-side dry-run for both Helm and Kubernetes resources. |
| `--skip-preload` | Skip image preloading even when `images.preload` is configured. |

## gck delete

Tear down a cluster and clean up all associated resources: the Kind cluster, load balancer containers, DNS records, mirror proxies, and the preload registry.

```bash
gck delete
gck delete my-cluster
```

### How it works

1. **Resolve target** -- gck determines which cluster to delete (see [Target resolution](#target-resolution) below).
2. **DNS records** -- Removes the cluster's DNS record file.
3. **Load balancer containers** -- Stops and removes Docker containers created by the cloud provider controller.
4. **Kind cluster** -- Deletes the Kind cluster, removing all namespaces, Helm releases, and applied manifests.
5. **Image mirrors** -- Stops mirror proxy containers if mirrors were configured.
6. **Preload registry** -- Stops the preload registry container if preloading was configured.
7. **Background processes** -- Stops the cloud provider controller and DNS server when no clusters or DNS records remain.
8. **State file** -- Removes the cluster's state file from `$GCK_HOME/clusters/`.

### Target resolution

`gck delete` doesn't need the original `gck.yaml` or registry to be available. It uses state files to find and clean up the cluster. See [Directory Layout -- clusters/]({{< ref "/docs/reference/directory-layout#clusters" >}}) for how target resolution and best-effort cleanup work.

## gck list

List all gck-managed clusters with their status.

```bash
gck list
```

Shows a table with the cluster name, creation date, context paths, active context flags, and whether the cluster is currently running.

### Example output

```
NAME                 CREATED            FROM                                  FLAGS              STATUS
kind-gravitee-apim   2026-03-23 14:00   gravitee-io/oss/apim/jdbc/postgres    --disable-es       running
kind-gravitee-apim   2026-03-22 10:30   gravitee-io/ee/apim/jdbc/postgres     -                  stopped
```

## gck describe

Show detailed information about a cluster: features, active load balancers, and DNS state. Information is read from the persisted cluster state, not from the current config.

```bash
gck describe
gck describe my-cluster
```

When no name is given and only one cluster exists, it is selected automatically. When multiple clusters exist, gck asks you to specify one (use `gck list` to see them).

### What it shows

**Cluster** -- The cluster name, creation date, context paths, and any context flags that were active at creation time.

**Features** -- Whether load balancers, Gateway API, and DNS are enabled. For Gateway API, shows the channel (`standard` or `experimental`). For DNS, shows the domain and port.

**Load Balancers** -- Lists active load balancer containers and their IPs. gck queries Docker directly for containers associated with the cluster, so this reflects the actual running state even if the original config is no longer available.

**DNS** -- Three pieces of information:
- **Resolver**: whether OS-level DNS routing is configured (i.e. whether `gck setup dns` has been run)
- **Server**: whether the local DNS server process is running
- **Records**: all registered hostname-to-IP mappings, grouped by cluster

### Example output

```
Cluster
  Name:    gio-apim
  Created: 2026-03-23 14:00
  From:    gravitee-io/oss/apim/jdbc/postgres
  Flags:   --disable-es

Features
  lb:      enabled
  gateway: disabled
  dns:     enabled (domain: gck.local, port: 15353)

Load Balancers
  gck-lb-gio-apim-1 → 172.18.0.5

DNS
  resolver: configured for gck.local
  server:   running on 127.0.0.1:15353
  records:
    console.gck.local → 172.18.0.5 (gio-apim)
    gateway.gck.local → 172.18.0.5 (gio-apim)
```

## gck validate

Validate `gck.yaml` and context flag files (`gck--*.yaml`) against the configuration schema. Catches typos, unknown fields, and type mismatches before you deploy.

```bash
gck validate
gck validate registry/kafka/standalone/gck.yaml
gck validate registry/
```

When given a directory, gck walks it recursively and validates every `gck.yaml` and `gck--*.yaml` file it finds. Context flag files are additionally checked for a valid naming convention and a non-empty `description` field. When no argument is given, it validates `./gck.yaml` in the current directory.

Exit code is non-zero when any file fails validation, making it suitable for CI pipelines and pre-commit checks.

### Flags

This command has no additional flags beyond the [global flags](#global-flags).

## gck setup dns

Configure your operating system to forward `*.gck.local` queries to the local DNS server. This is a one-time setup that requires sudo.

```bash
gck setup dns
```

- **macOS**: creates `/etc/resolver/gck.local` (persists across reboots)
- **Linux**: configures `systemd-resolved` on the loopback interface (runtime only)

After this, `gck create` and `gck delete` run without sudo.

## gck teardown dns

Remove the OS-level DNS routing created by `gck setup dns`.

```bash
gck teardown dns
```

## gck refresh dns

Re-collect DNS records from the running cluster. Use this after deploying additional Gateways or LoadBalancer Services that weren't present during `gck create`.

```bash
gck refresh dns
```

The running DNS server picks up the updated records immediately.

## Global flags

These flags are available on all commands:

| Flag | Description |
|------|-------------|
| `--config <path>` | Project-level config file to merge on top of the user-level base (`$GCK_HOME/gck.yaml`). Defaults to `./gck.yaml` when present. |
| `--registry <url>` | Registry URL (e.g. `file://./registry` or `https://…`). Overrides the value from config. |
| `--from <path>` | Context path to compose (e.g. `elastic/elasticsearch/standalone`). Repeatable. Overrides the `from` list from config. |
| `--set <key=value>` | Set a template variable. Repeatable. Overrides defaults declared in the `vars` block of any gck.yaml. See [Template variables](#template-variables). |

## Template variables

gck.yaml files support Go template expressions. Declare variables with defaults in a `vars` block and reference them with `{{ .variableName }}`:

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
        ui:
          image:
            tag: "{{ .imageTag }}"
```

Override at deploy time with `--set`:

```bash
gck create --set imageTag=4.12.0
gck patch --set imageTag=4.12.0 --set helmVersion=4.12.0
gck patch upgrade.yaml --set imageTag=4.12.0
```

### Precedence

1. `vars` defaults in the gck.yaml file (lowest)
2. `--set` values from the CLI (highest)

### Template functions

| Function | Usage | Description |
|----------|-------|-------------|
| `env` | `{{ env "HOME" }}` | Returns the value of an environment variable. |
| `default` | `{{ .myVar \| default "fallback" }}` | Returns the fallback when the pipeline value is empty. |
| `required` | `{{ .myVar \| required "myVar must be set" }}` | Returns the value or fails with the given message when empty. |

### Scope

Templating applies to every gck.yaml in the pipeline: user config, `$GCK_HOME/gck.yaml`, patch files, context flag overlays, and registry context files. Each file is templated independently with its own `vars` defaults merged with the shared `--set` overrides.
