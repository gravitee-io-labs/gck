---
title: "httpbin"
description: "HTTP request and response service for testing gateways and clients with zero setup"
tags: [sink]
---

# httpbin

Deploys [go-httpbin](https://github.com/mccutchen/go-httpbin), a Go
implementation of the httpbin API, into a local Kind cluster. Every
endpoint answers without configuration: echo requests, force status codes,
add latency, stream responses, and exercise auth schemes. Use it as the
upstream for e2e tests that check what a gateway or client sends and how it
reacts to known responses.

- Endpoint: `http://localhost:31880` from the host,
  `http://httpbin:8080` from inside the cluster.
- Web UI listing every endpoint: `http://localhost:31880`.

Need an upstream that misbehaves in scripted, per-test ways instead? Use
`mockserver/standalone`.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from httpbin/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

Echo back the method, headers, query and body of any request:

```bash
curl -s -X POST 'http://localhost:31880/anything/some/path?q=1' \
  -H 'X-Test: hello' -d '{"key":"value"}'
```

Force a status code, or pick one at random from a weighted list:

```bash
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:31880/status/503
```

```bash
curl -s -o /dev/null -w '%{http_code}\n' 'http://localhost:31880/status/200:0.8,503:0.2'
```

Add latency to test timeouts:

```bash
curl -s http://localhost:31880/delay/3
```

Other useful endpoints: `/headers`, `/ip`, `/basic-auth/{user}/{pass}`,
`/bearer`, `/redirect/{n}`, `/stream/{n}`, `/sse`, `/websocket/echo` and
`/hostname`. See the
[go-httpbin documentation](https://github.com/mccutchen/go-httpbin#readme)
for the full list.
