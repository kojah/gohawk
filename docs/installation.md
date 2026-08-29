---
title: Installation
description: Install gohawk and run your first analysis.
---

gohawk requires Go 1.26 or newer.

## Install the CLI

Install the latest release with Go:

```sh
go install github.com/kojah/gohawk@latest
```

Go writes the executable to `GOBIN`, or to `GOPATH/bin` when `GOBIN` is not
set. Make sure that directory is included in your `PATH`.

Verify the installation:

```sh
gohawk -V
```

## Run gohawk

Run the default analyzer profile across the current module:

```sh
gohawk ./...
```

gohawk can also run as a `go vet` tool:

```sh
go vet -vettool="$(command -v gohawk)" ./...
```

To include gohawk in a custom golangci-lint binary instead, follow the
[golangci-lint integration guide](../golangci-lint/).

Continue to [Configuring gohawk](/gohawk/configuration/) to select analyzers, enable
opt-in checks, and configure suppressions.
