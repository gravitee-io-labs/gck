---
title: "Installation"
weight: 3
type: docs
---

## Homebrew

```bash
brew tap gravitee-io-labs/gck https://github.com/gravitee-io-labs/gck
brew trust gravitee-io-labs/gck
brew install gck
```

> Homebrew 6.0+ requires third-party taps to be explicitly trusted before
> their formulas can be installed. The `brew trust` step above authorizes
> Homebrew to load the gck formula.

## Linux packages

Download the `.deb` or `.rpm` from the [latest release](https://github.com/gravitee-io-labs/gck/releases/latest):

```bash
# Debian / Ubuntu
sudo dpkg -i sew_*_linux_amd64.deb

# Fedora / RHEL
sudo rpm -i sew_*_linux_amd64.rpm
```

## go install

```bash
go install github.com/gravitee-io-labs/gck@latest
```

Requires Go 1.25+.

## From source

```bash
git clone https://github.com/gravitee-io-labs/gck.git
cd gck
go build -o gck .
```
