<p align="center"><img src="logo.svg" alt="gck" width="140"/><br><em>The Gravitee Cluster Kit</em></p>

<p align="center">
<a href="https://github.com/gravitee-io-labs/gck/actions/workflows/lint.yml"><img src="https://github.com/gravitee-io-labs/gck/actions/workflows/lint.yml/badge.svg" alt="CI"></a>
<a href="https://github.com/gravitee-io-labs/gck/releases/latest"><img src="https://img.shields.io/github/v/release/gravitee-io-labs/gck" alt="Release"></a>
<img src="https://img.shields.io/github/go-mod/go-version/gravitee-io-labs/gck" alt="Go version">
<img src="https://img.shields.io/github/license/gravitee-io-labs/gck" alt="License">
<a href="https://goreportcard.com/report/github.com/gravitee-io-labs/gck"><img src="https://img.shields.io/badge/go%20report-A+-brightgreen.svg?style=flat" alt="Go Report Card"></a>
</p>

<p align="center">
<a href="#quick-start">Quick Start</a> · <a href="https://gravitee-io-labs.github.io/gck/docs/">Documentation</a> · <a href="https://gravitee-io-labs.github.io/gck/registry/">Registry</a>
</p>

## Quick start

Install gck and make sure [Docker](https://docs.docker.com/get-docker/) is running:

```bash
go install github.com/gravitee-io-labs/gck@latest
```

Pick a context from the registry and deploy it in one command:

```bash
gck create --from gravitee-io/oss/apim
```

That's it — gck creates a Kind cluster, installs all components, and gives you a full Gravitee API Management stack. When you're done:

```bash
gck delete
```

## Features

- **Discoverable** — Browse available contexts in the [registry](https://gravitee-io-labs.github.io/gck/registry/) and deploy them with a single command.
- **Composable** — Contexts build on each other via `from`. Mix databases, brokers, and applications into a tailored stack without duplicating configuration.
- **Wired** — Load balancers, Gateway API, DNS proxying — production-like networking on localhost, out of the box.
- **Automatable** — JSON Schema, structured registry, and machine-readable config make gck a first-class target for automation — from CI pipelines to AI assistants.

## Prerequisites

- **Go 1.25+**
- **Docker**

## Contributing

### Developers

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full guide. Key commands:

```bash
go tool task lint          # run Go linter
go tool task test          # run all tests
go tool task fmt:yaml      # format YAML files
go tool task site:build    # rebuild the doc site after registry/ or site/ changes
```

### Context maintainers

Contexts live under `registry/` following the `org/edition/product/variant` convention. Each context has a `gck.yaml` describing Helm repos, components, and features. Refer to the [Context Format](https://gravitee-io-labs.github.io/gck/docs/reference/context-format/) and [AI Toolchain](https://gravitee-io-labs.github.io/gck/docs/guides/ai-toolchain/) docs for authoring guidelines.

## License

[Apache License 2.0](LICENSE)
