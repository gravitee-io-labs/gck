---
title: "Kafka"
description: "Single-node Kafka broker in KRaft combined mode"
tags: [messaging]
---

# Kafka

Deploys a single-node Apache Kafka broker running in KRaft combined mode
(no ZooKeeper) with host access on port 30092.

## Install gck

```bash
go install github.com/gravitee-io-labs/gck@latest
```

For other installation methods, see [Installation](https://gravitee-io-labs.github.io/gck/docs/getting-started/installation/).

## Usage

### Create

```bash
gck create --from kafka/standalone
```

### Cleanup

```bash
gck delete
```

## Quick Start

List topics from your host using [kcat](https://github.com/edenhill/kcat):

```bash
kcat -b 127.0.0.1:30092 -L
```

Produce and consume a test message:

```bash
echo "hello" | kcat -b 127.0.0.1:30092 -P -t test-topic
kcat -b 127.0.0.1:30092 -C -t test-topic -e
```

| Parameter | Value     |
|-----------|-----------|
| Bootstrap | 127.0.0.1:30092 |
| Protocol  | PLAINTEXT |
