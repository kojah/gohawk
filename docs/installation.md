---
title: Installation
description: Install gohawk and run your first analysis.
---

gohawk requires Go 1.26 or newer to build. It officially supports running
against the Go 1.26 and Go 1.27 commands. Official release binaries are built
with Go 1.27 so the same binary supports both versions. Projects may target
older language versions through their `go` directive, but older Go commands
are best effort.

## Install the CLI

Install the latest release with Go:

```sh
go install github.com/kojah/gohawk@latest
```

For a reproducible toolchain or CI setup, replace `vX.Y.Z` with the release
you want to pin:

```sh
go install github.com/kojah/gohawk@vX.Y.Z
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

Reinstall gohawk after upgrading the Go toolchain used by the project. A
locally built analysis binary is supported only for Go versions no newer than
the toolchain that built it.

## Run gohawk

Run gohawk's conservative default set across the current module:

```sh
gohawk ./...
```

gohawk can also run as a `go vet` tool:

```sh
go vet -vettool="$(command -v gohawk)" ./...
```

Analysis exits with status 0 when no findings remain, status 3 when findings
are reported, and status 1 for build or analysis failures. `-json` changes the
output format without changing those enforcement semantics.

To include gohawk in a custom golangci-lint binary instead, follow the
[golangci-lint integration guide](../golangci-lint/).

Continue to [Configuration](/configuration/) to select analyzers, raise the
tier ceiling, and configure suppressions.

## Optional: pre-commit

If you use pre-commit, add gohawk to `.pre-commit-config.yaml` with a pinned
release:

```yaml
repos:
  - repo: https://github.com/kojah/gohawk
    rev: vX.Y.Z
    hooks:
      - id: gohawk
```

The hook uses pre-commit's isolated Go environment and caches the compiled
binary. Add analyzer flags through `args`, keeping `./...` as the final
argument:

```yaml
      - id: gohawk
        args: [-json, -enable-all, ./...]
```
