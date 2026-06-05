---
title: "Troubleshooting"
weight: 8
type: docs
---

Common issues and how to fix them.

## Port conflict between two clusters

**Symptom:** `gck create` fails with `command docker run failed with error: exit status 125` when creating a second cluster.

**Cause:** Both clusters use contexts that bind the same host ports via Kind's `extraPortMappings` (e.g. two API gateway stacks both claiming port 80 and 443). Docker can't bind the same host port twice.

**Fix:** Override the port mappings in your `gck.yaml` for the second cluster to use different host ports. You also need to update the service configuration (NodePort values, base URLs) in the component's Helm values to match:

```diff
 kind:
   name: second-cluster
   nodes:
     - role: control-plane
       extraPortMappings:
         - containerPort: 30080
-          hostPort: 30080
+          hostPort: 31080
         - containerPort: 30082
-          hostPort: 30082
+          hostPort: 31082
 
 components:
   - name: apim
     helm:
       values:
         ui:
           service:
-            nodePort: 30080
+            nodePort: 31080
         gateway:
           services:
             core:
               service:
-                nodePort: 30082
+                nodePort: 31082
```

Both sides must stay in sync: `extraPortMappings` controls which ports Kind exposes on the host, `nodePort` controls which ports the services bind inside the cluster.

Alternatively, delete the first cluster before creating the second one:

```bash
gck delete <first-cluster>
```

To see which ports are already bound by Docker containers:

```bash
docker ps --format "table {{.Names}}\t{{.Ports}}"
```

## DNS not resolving after `gck create`

**Symptom:** `curl api.gck.local` times out or returns NXDOMAIN.

**Cause:** OS-level routing was never configured (one-time `gck setup dns` step).

**Fix:** Run the DNS setup (requires sudo) and check status afterwards:

```bash
gck setup dns
gck describe
```

The DNS section of `gck describe` shows resolver, server, and records state. See the [Networking guide]({{< ref "/docs/guides/networking/#local-dns" >}}) for details.

## `sudo` prompt on macOS when creating a cluster

**Symptom:** macOS asks for your password during `gck create` with load balancers enabled.

**Cause:** Docker on macOS runs in a VM; gck sets up a packet tunnel to make container IPs routable from the host, which requires root.

**Fix:** This is expected. Enter your password. Subsequent `gck create` calls in the same session won't re-prompt unless the tunnel was torn down.

## `gateway requires lb, but lb is explicitly disabled`

**Symptom:** `gck create` fails with this validation error.

**Cause:** Gateway API needs load balancers to assign IPs to Gateways. The config has `features.gateway.enabled: true` but `features.lb.enabled: false`.

**Fix:** Either enable load balancers explicitly:

```yaml
features:
  lb:
    enabled: true
```

Or let gck auto-enable it by removing the explicit `lb` block entirely.

## Component not ready (timeout)

**Symptom:** `gck create` hangs and eventually fails with "timeout waiting for X to be ready."

**Cause:** A pod isn't reaching Ready state within the timeout (default 5 min). Common reasons: image pull failure, crash loop, missing dependency, resource limits.

**Fix:** Start with the gck install log -- it captures Helm and kubectl output that isn't shown in the terminal. See [Directory Layout -- logs/]({{< ref "/docs/reference/directory-layout#logs" >}}) for the file location. Then check pod status:

```bash
kubectl get pods -A
kubectl logs -n <ns> <pod>
```

If the component just needs more time, increase its timeout in your `gck.yaml`:

```yaml
components:
  - name: apim
    timeout: 15m
```

The same field is available on dependency entries (`requires[].timeout`) to give a specific upstream more time. Check `gck describe` for image preloading or mirror issues.

## Image pull errors / Docker Hub rate limiting

**Symptom:** Pods stuck in `ImagePullBackOff` or `ErrImagePull`. Docker Hub returns 429 (Too Many Requests).

**Cause:** Anonymous Docker Hub pulls are rate-limited (100 pulls / 6 hours). Heavy image contexts can hit this on first run.

**Fix:** Enable image mirrors to cache layers locally. Mirrors run pull-through `registry:2` containers on your machine that survive cluster deletions, so layers are only downloaded once:

```yaml
images:
  mirrors:
    upstreams:
      - ghcr.io
      - docker.elastic.co
```

An empty `mirrors: {}` mirrors `docker.io` only (the default upstream). Adding `upstreams` extends the list with additional registries.

For authenticated pulls, configure `~/.docker/config.json` — gck's mirror proxies forward credentials. See the [Container Images guide]({{< ref "/docs/guides/container-images/#image-mirrors" >}}) for the full reference.

## Stale cluster after a crash

**Symptom:** `gck create` fails because the Kind cluster already exists, or Docker containers from a previous run are still around.

**Cause:** A previous `gck delete` didn't complete (crash, Ctrl+C, Docker restart).

**Fix:** Run a clean teardown (check `$GCK_HOME/logs/delete.log` if it fails -- see [Directory Layout]({{< ref "/docs/reference/directory-layout#logs" >}})):

```bash
gck delete <cluster>
```

If no state file exists, gck does best-effort cleanup. As a last resort:

```bash
kind delete cluster --name <name>
docker rm -f $(docker ps -aq --filter "name=gck-mirror-")
docker rm -f gck-preload
```

If things went really bad, gck may also have left background processes running (DNS server, cloud-provider-kind controller). Check for them:

```bash
pgrep -af "gck.*(dns|cpk) serve"
```

Kill any orphaned processes:

```bash
kill $(pgrep -f "gck.*(dns|cpk) serve") 2>/dev/null
```

> On macOS, the cloud-provider controller runs as root. Use `sudo kill` if `kill` returns "Operation not permitted".

Then clean up stale PID files:

```bash
rm -f ~/.gck/pids/dns.pid ~/.gck/pids/cpk.pid
```
