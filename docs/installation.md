---
title: Installation
description: Install gohawk and run your first analysis.
---

gohawk requires Go 1.26 or newer to build. It officially supports running
against the Go 1.26 and Go 1.27 commands. Official release binaries are built
with Go 1.27 so the same binary supports both versions. Projects may target
older language versions through their `go` directive, but older Go commands
are best effort.

Using VS Code or Cursor? The [gohawk extension](/vscode/) includes the binary, so you can
skip this page.

## Install the CLI

Download an archive for your platform from
[GitHub Releases](https://github.com/kojah/gohawk/releases), extract the executable,
and put it on your `PATH`. Archives cover Linux, macOS, and Windows on AMD64 and
ARM64. Each release includes SHA-256 checksums. The prebuilt CLI still requires
a supported Go command to analyze projects.

Generated Go sources are committed, so v0.8.0 installs directly with the Go
command:

```sh
go install github.com/kojah/gohawk@v0.8.0
```

v0.6.0 and earlier releases did not commit generated sources and cannot be built
this way. Development builds that change rules regenerate through Make, which
requires Go 1.27. Make downloads and builds the pinned generators automatically:

```sh
git clone https://github.com/kojah/gohawk.git
cd gohawk
make build
```

The executable is `.build/gohawk`. Copy it to a directory on your `PATH`.
To build a particular revision, check it out before running `make build`.

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

Install the CLI first and keep `gohawk` on `PATH` wherever pre-commit runs,
including CI. The hook runs that installed binary; it does not compile GoHawk.
Add analyzer flags through `args`, keeping `./...` as the final argument:

```yaml
      - id: gohawk
        args: [-json, -enable-all, ./...]
```
