---
title: "Gamma"
description: "Gravitee Gamma with Access Management and MongoDB backend"
tags: [networking, security]
---

# Gamma

Deploys a full Gravitee Gamma stack alongside Access Management, backed
by MongoDB for persistence and Elasticsearch for analytics.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from gravitee-io/ee/gamma
```

### Cleanup

```bash
gck delete
```

## Quick Start

Sign in to the Gamma Console at [http://localhost:30085](http://localhost:30085)
with the default admin account (`admin` / `admin`).

The APIM Console is available at [http://localhost:30080](http://localhost:30080)
with the same credentials, and the AM Console at
[http://localhost:30090](http://localhost:30090) (`admin` / `adminadmin`).

To connect Gamma to Access Management:

1. Open the AM Console at [http://localhost:30090](http://localhost:30090) and create a service account token.
2. Head to the platform module in the Gamma Console at [http://localhost:30085](http://localhost:30085).
3. Use `http://am-management-api:83` as the AM URL and paste the token.

To get started with Gravitee API Management, follow the
[APIM quick start guide](https://documentation.gravitee.io/apim/getting-started/quickstart-guide).

## Endpoints

| Service            | URL                   |
|--------------------|-----------------------|
| Gamma Console      | http://localhost:30085 |
| APIM Console       | http://localhost:30080 |
| APIM Portal        | http://localhost:30081 |
| APIM Gateway       | http://localhost:30082 |
| Management API     | http://localhost:30083 |
| Edge Gateway       | http://localhost:30086 |
| AM Console         | http://localhost:30090 |
| AM Gateway         | http://localhost:30092 |
| AM Management API  | http://localhost:30093 |

## License

This is an Enterprise Edition (EE) context. Place your Gravitee license
key at `$HOME/opt/gravitee/license.key` and gck will automatically mount
it into the cluster. If the file is missing, the license component is
silently skipped (`onMissing: ignore`).

To use a different path, override it in your `gck.yaml`:

```yaml
components:
  - name: license
    k8s:
      secrets:
        - name: gravitee-license
          fromFile: '/custom/path/to/license.key'
```
