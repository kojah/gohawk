---
title: Installation
description: Install gohawk and run your first analysis.
---

gohawk requires Go 1.27 or newer to build. It officially supports running
against the Go 1.26 and Go 1.27 commands using the same Go 1.27-built binary.
Projects may target older language versions through their `go` directive, but
older Go commands are best effort.

## Install the CLI

Install the latest release with Go:

```sh
go install github.com/kojah/gohawk@latest
```

For a reproducible toolchain or CI setup, pin the current stable release:

```sh
go install github.com/kojah/gohawk@v0.3.1
```

Prebuilt archives for Linux, macOS, and Windows on AMD64 and ARM64 are also
available from [GitHub Releases](https://github.com/kojah/gohawk/releases).
Each release includes SHA-256 checksums for verifying the downloads. The
prebuilt CLI still requires a supported Go command to analyze projects.

Go writes the executable to `GOBIN`, or to `GOPATH/bin` when `GOBIN` is not
set. Make sure that directory is included in your `PATH`.

Verify the installation:

```sh
gohawk -V
```

Reinstall gohawk after upgrading the Go toolchain used by the project. Go
analysis binaries must be built with a toolchain at least as new as the code
and standard library they analyze.

## Run gohawk

Run gohawk's conservative default set across the current module:

```sh
gohawk ./...
```

gohawk can also run as a `go vet` tool:

```sh
go vet -vettool="$(command -v gohawk)" ./...
```

To include gohawk in a custom golangci-lint binary instead, follow the
[golangci-lint integration guide](../golangci-lint/).

Continue to [Configuration](/configuration/) to select analyzers, raise the
tier ceiling, and configure suppressions.
