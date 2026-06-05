---
title: "CI/CD"
weight: 6
type: docs
---

This guide covers the basics of running gck in a CI pipeline, image caching strategies for different platforms, and complete configuration examples for CircleCI and GitHub Actions.

## Integration testing

A typical integration test pipeline creates a cluster, runs tests against it, and tears it down:

```bash
gck create --from gravitee-io/oss/apim/dbless
make test
gck delete gravitee-dbless
```

## Private registries

If your pipeline uses a private gck registry, write a `~/.netrc` file with the registry host's credentials before running `gck create`. gck reads `.netrc` automatically -- no extra flags needed:

```bash
echo "machine registry.mycompany.com login deploy password ${REGISTRY_TOKEN}" > ~/.netrc
chmod 600 ~/.netrc
```

Store the token as a CI secret (`REGISTRY_TOKEN` above). See [Private registry authentication]({{< ref "/docs/guides/gck-registry#private-registry-authentication" >}}) for details on the `.netrc` format.

## Speeding up image pulls

Image pulls are usually the slowest part of cluster creation. gck offers two strategies -- **preload** and **mirrors** -- and the right choice depends on whether your CI platform provides Docker Layer Caching (DLC). See the [Container Images]({{< ref "/docs/guides/container-images" >}}) guide for the full reference on both strategies.

### Preload + DLC

Image preloading works by running `docker pull` on the host daemon for every image in the `images.preload.refs` list, then pushing them to a local registry that Kind nodes pull from. The host `docker pull` is the expensive, network-bound step.

On CI platforms that offer Docker Layer Caching -- such as CircleCI -- the host daemon's layer store is persisted between runs. Once an image is cached by DLC, subsequent pulls are effectively free: DLC serves the layers from its cache instead of hitting the upstream registry. This makes preload the simplest and fastest CI image strategy on those platforms, with no explicit cache configuration needed from you.

> Caching `$GCK_HOME/preload/` on top of DLC is not worth the complexity. The preload directory backs the local registry (the fast local-push step), not the host pulls (the slow step that DLC already handles).

### Mirrors + cache

On platforms without DLC, preload's host `docker pull` hits the network on every run and there is nothing gck-side to cache it. A better strategy here is **image mirrors**.

Mirror proxies are pull-through `registry:2` containers that cache layers in `$GCK_HOME/mirrors/`. Kind nodes pull through the mirrors instead of going to upstream registries -- and the host Docker daemon is not involved at all. By caching the `$GCK_HOME/mirrors/` directory between CI runs, mirror containers start with a warm layer cache and serve images locally:

1. Restore `$GCK_HOME/mirrors/` from the CI cache
2. `gck create` starts mirror containers mounted to the cached data directory
3. Kind nodes pull through mirrors -- cached layers are served locally
4. Save `$GCK_HOME/mirrors/` for the next run

The first run still pulls everything from upstream, but every subsequent run benefits from the cached layers.

## CircleCI

CircleCI's `machine` executor gives you a full VM with Docker pre-installed -- exactly what Kind needs. More importantly, CircleCI offers **Docker Layer Caching**, which caches the host Docker daemon's layer store between runs. This makes `images.preload` the ideal strategy: gck pulls images via `docker pull` on the host, and DLC ensures those pulls are near-instant on repeat jobs.

```yaml
version: 2.1

orbs:
  go: circleci/go@3.0.3

jobs:
  test:
    machine:
      image: ubuntu-2404:current
      docker_layer_caching: true
    steps:
      - checkout
      - go/install:
          version: "1.25.9"
      - run:
          name: Install gck
          command: go install github.com/gravitee-io-labs/gck@latest
      - run:
          name: Create cluster
          command: gck create --from gravitee-io/oss/apim/dbless
      - run:
          name: Verify
          command: gck describe gravitee-dbless
      - run:
          name: Delete cluster
          command: gck delete gravitee-dbless
          when: always

workflows:
  ci:
    jobs:
      - test
```

No explicit cache configuration is needed. The dbless context already defines `images.preload.refs`, and DLC caches the host `docker pull` layers transparently. The local push to the preload registry is fast regardless.

## GitHub Actions

GitHub Actions runners have Docker pre-installed but do not offer Docker Layer Caching for image pulls. Use **mirrors** with `actions/cache` to avoid re-downloading images on every run.

```yaml
name: Integration tests

on: [push]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: stable
      - run: go install github.com/gravitee-io-labs/gck@latest

      - name: Restore mirror cache
        uses: actions/cache@v4
        with:
          path: ~/.gck/mirrors
          key: gck-mirrors-${{ hashFiles('gck.yaml') }}
          restore-keys: gck-mirrors-

      - name: Create cluster
        run: gck create --from gravitee-io/oss/apim/dbless

      - name: Verify
        run: gck describe gravitee-dbless

      - name: Delete cluster
        if: always()
        run: gck delete gravitee-dbless
```

The `gck.yaml` for this pipeline would use mirrors instead of preload:

```yaml
from:
  - gravitee-io/oss/apim/dbless

images:
  mirrors: {}
```

## Validating your own registry

If you maintain your own registry of contexts, add a validation step to catch schema errors on every push. `gck validate` checks `gck.yaml` and context flag files (`gck--*.yaml`) against the configuration schema -- typos, unknown fields, type mismatches, and missing flag descriptions all produce a non-zero exit code:

```bash
gck validate registry/
```

When given a directory, gck walks it recursively and validates every context and flag file it finds. Add this as a standalone CI job or a pre-merge check -- it runs in seconds and doesn't need Docker. See the [validate command reference]({{< ref "/docs/reference/commands#gck-validate" >}}) and the [Context Format]({{< ref "/docs/reference/context-format" >}}) reference for authoring guidelines.

For the full reference on image strategies, see the [Container Images]({{< ref "/docs/guides/container-images" >}}) guide. For all CLI flags and commands mentioned here, see the [Commands]({{< ref "/docs/reference/commands" >}}) reference.

## Debugging

When something fails, check `$GCK_HOME/logs/`. Helm and kubectl output that isn't shown in the terminal is captured there. Start with the install log for the context -- it usually contains the error that explains why a component failed. See [Directory Layout -- logs/]({{< ref "/docs/reference/directory-layout#logs" >}}) for the full layout.