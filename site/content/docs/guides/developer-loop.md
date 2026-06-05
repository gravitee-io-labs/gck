---
title: "Developer Loop"
weight: 5
type: docs
---

When you're iterating on application code, the last thing you want is a slow feedback loop. gck's `builds` feature lets you compile, build a Docker image, push it to the cluster, and restart workloads -- all in a single command, without leaving your terminal.

## Defining builds

Add a `builds` section to your `gck.yaml`. Each entry describes one image you build locally. Here is a real-world example that builds Ambassador Edge Stack from source:

```yaml
from:
  - gravitee-io/ee/edge-stack

builds:
  - name: emissary
    image: emissary-base:local
    dir: '{{ env "HOME" }}/src/gravitee/edge-stack'
    pre:
      - EMISSARY=$(make -C apro env BUILD_VERSION=dev 2>/dev/null | sed -n 's/^EMISSARY_IMAGE=//p') && docker pull --platform linux/amd64 $EMISSARY && docker tag $EMISSARY emissary-base:local
  - name: aes
    image: docker.io/datawire/aes:3.12.7
    dir: '{{ env "HOME" }}/src/gravitee/edge-stack/apro'
    pre:
      - make $PWD/vendor
    platform: linux/amd64
    buildArgs:
      EMISSARY_BASE: emissary-base:local
```

The `from` field pulls the edge-stack registry context, which brings in the Kind cluster config, Helm chart, CRDs, and preload images. The `builds` section adds two local builds on top.

Builds run sequentially in declaration order, which matters here because the two entries form a chain:

1. **`emissary`** has no Dockerfile in its context directory. gck detects this and skips `docker build`, but still runs the `pre` command -- which pulls the upstream Emissary image for `linux/amd64` and tags it as `emissary-base:local` -- then pushes that image to the cluster. This pattern is useful for preparing base images from upstream without writing a Dockerfile.

2. **`aes`** builds the actual Edge Stack image. It references the locally-tagged base via `buildArgs.EMISSARY_BASE`, vendorizes Go dependencies in its `pre` step, and forces `platform: linux/amd64` so the build works on Apple Silicon machines where the base image is only published for amd64.

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Short identifier, used to select builds on the CLI |
| `image` | yes | Docker image tag to build (e.g. `myapp:latest`) |
| `dir` | no | Working directory for `pre` commands and base for relative paths. Use `{{ env "HOME" }}` for env vars. Defaults to `.` |
| `pre` | no | Shell commands run sequentially before `docker build` (compilation, packaging, etc.) |
| `buildArgs` | no | Docker build arguments passed to `docker build --build-arg`. Use `{{ env "VAR" }}` for env-var expansion |
| `context` | no | Docker build context, relative to `dir`. Defaults to `.` |
| `dockerfile` | no | Path to the Dockerfile, relative to `dir`. Defaults to `Dockerfile` in the context |
| `platform` | no | Target platform for `docker build --platform` (e.g. `linux/amd64`). Useful when the base image is only available for a specific architecture |

> When neither `dockerfile` is set nor a `Dockerfile` exists in the resolved context directory, gck skips `docker build` entirely but still pushes the image to the cluster and restarts workloads. Use this for pre-only builds that pull or tag images without building them.

## Running builds

```bash
gck build
```

This builds every entry in declaration order, pushes each image to the cluster's preload registry, and restarts any Deployment or StatefulSet that references the image. In the example above, `gck build` runs `emissary` first (pull + tag + push) then `aes` (vendorize + docker build + push). Build output is captured in `~/.gck/logs/build/build.log` and the terminal shows a clean progress view.

### Building a subset

Pass one or more names to build only what you need:

```bash
gck build aes
```

Once the Emissary base image is cached in the cluster, you typically only rebuild `aes` during iteration. You can also pass multiple names:

```bash
gck build emissary aes
```

### Skipping pre-build commands

When you've already vendorized locally and just want to rebuild the Docker image:

```bash
gck build --skip-pre aes
```

### Building without restarting

Push the image to the registry but don't trigger a rollout restart:

```bash
gck build --no-restart aes
```

## Creating and building in one step

If you don't have a cluster yet, `--create` creates one before building. When the cluster already exists, the flag is silently ignored:

```bash
gck build --create
```

This runs the full `gck create` flow (preload, cluster, component install) then proceeds with the builds. You can pass context flags too -- they're forwarded to the creation step:

```bash
gck build --create --skip-pre aes
```

This is the fastest way to go from a clean machine to running your local code on a cluster.

## How builds interact with preloading

When `builds` is configured, gck automatically excludes build images from the preload list during `gck create`. The edge-stack context preloads `docker.io/datawire/aes:3.12.7`, but since the `aes` build targets the same image tag, gck skips pulling it from the remote registry -- it knows a local build will replace it.

This works transparently with both merge and replace preload modes. You don't need to manually add `skip` entries for your build images.

## Flags reference

| Flag | Description |
|------|-------------|
| `--create` | Create the cluster if it doesn't exist |
| `--skip-pre` | Skip pre-build commands, go straight to `docker build` |
| `--no-restart` | Build and push but don't restart workloads |
| `--name <cluster>` | Target a specific cluster (default: from config) |

## Build logs

All build output is written to `~/.gck/logs/build/build.log`. When a step fails, gck points you to the log file:

```
  ✗ Running pre-build commands (failed after 12.3s)
    See logs: /home/user/.gck/logs/build/build.log
```

## Patching a running cluster

`gck build` is for local code changes. When you need to bump an upstream image tag, change a Helm chart version, or tweak values on a running cluster without recreating it, use `gck patch`.

Write a patch file with the components you want to upgrade:

```yaml
components:
  - name: edge-stack
    helm:
      values:
        emissary-ingress:
          image:
            tag: 3.13.0
```

Then apply it:

```bash
gck patch upgrade.yaml
```

gck merges the patch into the resolved context and upgrades only the named components -- everything else is left untouched. You can preview the changes before applying them:

```bash
gck patch upgrade.yaml --dry-run
```

This runs a server-side dry-run and prints a colored diff of what would change.

See the [patch command reference]({{< ref "/docs/reference/commands#gck-patch" >}}) for the full set of flags and merge rules.
