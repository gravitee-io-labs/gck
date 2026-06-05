---
title: "Why gck?"
weight: 2
type: docs
---

gck came from day-to-day frictions faced by the Gravitee developer experience team while running local Kubernetes clusters for tests and demos. These are a few that kept coming back.

## Cluster setup is repetitive boilerplate

Every developer on the team had their own approach: a shell script here, a Kind config there, a README with a list of Helm commands to run in the right order. It worked, but it was fragile and nobody's setup matched anyone else's.

We wanted a single command that would produce a known-good cluster from a known-good definition. That's the core idea behind **registry contexts** -- curated stack descriptions that capture everything: Kind configuration, Helm charts, raw manifests, port mappings, networking. Pick one and deploy it:

```bash
gck create --from gravitee-io/oss/apim
```

No scripts, no "follow the README and hope it's up to date."

## Stacks are duplicated across teams

The same MongoDB setup appeared in half a dozen repos, each with slightly different Helm values. When someone found a better configuration, it didn't propagate. Bug fixes in one copy didn't reach the others.

gck solves this with **composable contexts**. A MongoDB context lives in one place in the registry. An application context pulls it in with `from` and layers its own components on top. Abstract base contexts capture shared configuration that concrete variants extend. Context flags toggle optional pieces without forking the whole stack.

The result: one source of truth for each building block, reused everywhere it's needed.

## Image caching requires manual plumbing

You can set up pull-through caches, pre-pull images, run a local registry -- the building blocks exist. But wiring them together, keeping them running across cluster lifecycles, and making sure every developer on the team has the same setup is real work that nobody wants to own.

gck handles all of it out of the box. **Image mirrors** are persistent pull-through cache containers that survive cluster restarts -- once an image is cached, it stays cached. **Preloading** pulls images on the host and pushes them to a local registry inside the cluster, so Kind nodes don't hit the network at all. No manual setup, no per-developer differences -- it just works from the first `gck create`.

## The edit-build-deploy loop is too slow

The inner dev loop on Kubernetes involves at least three manual steps: build the Docker image, load it into Kind, and trigger a rollout restart. Each step has its own flags and failure modes. It's the kind of friction that makes developers avoid testing on a real cluster.

**`gck build`** collapses the whole sequence into one command. Define your builds in `gck.yaml` -- source directory, pre-build commands, Dockerfile -- and gck handles the rest. It even skips preloading for images you build locally, since it knows you'll push a fresh copy.

## Config mistakes surface too late

A typo in a Helm value or a missing field in `gck.yaml` doesn't show up until Kind is running and the Helm install fails -- minutes into the process, after you've already waited for images to load.

**`gck validate`** checks your config against the embedded JSON Schema before anything starts. The same schema powers editor autocompletion and inline validation, so most mistakes never make it to the command line at all.

gck is the tool we wished we had from the start. If any of the above sounds familiar, give it a try -- the [Getting Started tutorial]({{< ref "/docs/getting-started" >}}) will get you to a running cluster in under a minute.
