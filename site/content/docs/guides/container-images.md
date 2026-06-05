---
title: "Container Images"
weight: 4
type: docs
---

Pulling container images over the network every time you recreate a cluster gets old fast. gck gives you two strategies to speed things up: **mirror proxies** that cache layers locally, and **image preloading** that stages images before the cluster starts. The [Architecture]({{< ref "/docs/reference/architecture#component-interactions" >}}) page shows how these fit into the overall system.

## Image mirrors

Mirror proxies run as local `registry:2` containers that cache image layers on your machine. When you delete and recreate a cluster, cached layers are reused -- no re-downloading.

### How it works

Each upstream registry gets its own pull-through cache container, bound to a local port. The Kind nodes' containerd is configured to check the mirror first. Mirror containers use a `restart: unless-stopped` policy, so they survive `gck delete` and keep their cache across cluster lifecycles.

### Enabling mirrors

Add `images.mirrors` to your `gck.yaml`:

```yaml
# Mirror docker.io only (always implicit)
images:
  mirrors: {}
```

```yaml
# Mirror docker.io + additional registries
images:
  mirrors:
    upstreams:
      - ghcr.io
      - docker.elastic.co
```

`docker.io` is always included -- you don't need to list it explicitly.

### Mirror options

| Field | Default | Description |
|-------|---------|-------------|
| `upstreams` | *(none)* | Additional registries to mirror (on top of `docker.io`) |
| `data` | `$GCK_HOME/mirrors` | Directory for cached layers and containerd host configs |

### Private registries

Combine mirrors with component value overrides to pull from a private registry through the local cache:

```yaml
images:
  mirrors:
    upstreams:
      - acme.example.com

components:
  - name: my-app
    helm:
      valueFiles:
        - values-private.yaml
```

## Image preloading

As an alternative (or complement) to mirrors, you can preload specific images into the cluster before components are deployed. This is especially useful on CI systems with Docker Layer Caching (DLC).

### How it works

1. Before the Kind cluster is created, gck pulls each image listed in `images.preload.refs` on the host Docker daemon.
2. A local `registry:2` container (`gck-preload`) is started, backed by persistent storage in `$GCK_HOME/preload`. Pre-pulled images are re-tagged and pushed to this registry.
3. Kind nodes are configured to check the preload registry first for each upstream referenced by the listed images.

Because the preload registry stores its data on the host filesystem, cached layers survive across cluster lifecycles. When you delete and recreate a cluster, only layers that have actually changed need to be re-pushed -- stable images like databases are available instantly. For mutable-tag images (snapshots, `latest`), gck re-pulls the upstream version and pushes only the changed layers.

### Enabling preloading

```yaml
images:
  preload:
    refs:
      - docker.io/library/mongo:7
      - docker.elastic.co/elasticsearch/elasticsearch:8.17.0
      - bitnami/redis:7.4
```

Context authors can ship the image list directly in the context `gck.yaml`, so you don't have to maintain it yourself.

### How preload merges across layers

When a context composes from parents via `from`, or when you override preload in your own config, the preload lists are **merged** by default -- your refs are unioned with the inherited ones. This is controlled by the `mode` field.

| Field | Default | Description |
|-------|---------|-------------|
| `mode` | `merge` | `merge` unions refs with inherited preload. `replace` discards inherited refs entirely. |
| `refs` | *(none)* | Image references to preload |
| `skip` | *(none)* | Image references to exclude from inherited preload (merge mode only) |

#### Merge mode (default)

In merge mode, refs from every layer are deduplicated and combined. If a context preloads `mongo:7` and you add `redis:7.4`, the cluster gets both:

```yaml
images:
  preload:
    refs:
      - bitnami/redis:7.4
```

Omit `mode` (or set it to `merge`) to keep this behavior.

#### Replace mode

When you need full control over exactly which images are preloaded, set `mode: replace`. This discards every ref inherited from parent contexts and uses only the refs you list:

```yaml
images:
  preload:
    mode: replace
    refs:
      - docker.io/library/mongo:7
      - bitnami/redis:7.4
```

#### Skipping inherited images

Sometimes you want to keep most of the inherited preload list but remove a few images -- for example, when a context flag disables a component whose images would otherwise be pulled for nothing. Use `skip` instead of switching to replace mode:

```yaml
images:
  preload:
    skip:
      - docker.elastic.co/elasticsearch/elasticsearch:8.17.0
```

`skip` entries accumulate across layers the same way `refs` do: if a parent context skips an image and a child adds another skip, both are excluded.

This is especially useful in context flags. A `--disable-es` flag that disables Elasticsearch can skip its image in the same patch file:

```yaml
description: "Disable Elasticsearch and analytics reporters"
images:
  preload:
    skip:
      - docker.elastic.co/elasticsearch/elasticsearch:8.17.0
components:
  - name: elasticsearch
    enabled: false
```

> `skip` is ignored when `mode` is set to `replace`, since replace already gives you an explicit list.

### Combining mirrors and preloading

Both strategies work together. Preloading handles your known images (fast, DLC-friendly), while mirrors transparently cache anything else pulled at runtime:

```yaml
images:
  preload:
    refs:
      - docker.io/library/mongo:7
      - bitnami/redis:7.4
  mirrors:
    upstreams:
      - docker.elastic.co
```

## Skipping preload at runtime

If a context defines preload images but you want to bypass preloading for a particular run, use `--skip-preload`:

```bash
gck create --skip-preload
gck patch upgrade.yaml --skip-preload
```

This is useful when mirrors already cover the upstreams you need, or during iterative development where you want a faster cluster startup. If you're building images locally as part of a dev loop, see the [Developer Loop]({{< ref "/docs/guides/developer-loop" >}}) guide.
