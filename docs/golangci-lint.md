---
title: GolangCI integration
description: Run gohawk as a module plugin in a custom golangci-lint binary.
---

gohawk integrates with golangci-lint through its
[module plugin system](https://golangci-lint.run/docs/plugins/module-plugins/).
Module plugins are compiled into a custom golangci-lint binary, so they do not
require a separate plugin file at runtime.

## Build a custom binary

Create `.custom-gcl.yml` in the root of your project:

```yaml
version: v2.13.2
name: custom-gcl
destination: .
plugins:
  - module: github.com/kojah/gohawk
    import: github.com/kojah/gohawk/plugin/golangci
    version: v0.2.1
```

Build the custom binary with Go 1.27 or newer. The build toolchain must be at
least as new as the code the resulting binary will analyze:

```sh
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 custom
```

This writes `custom-gcl` to the current directory.

## Configure gohawk

Enable the custom linter in `.golangci.yml`:

```yaml
version: "2"

linters:
  default: none
  enable:
    - gohawk
  settings:
    custom:
      gohawk:
        type: module
        description: Correctness-focused ownership, lifecycle, and concurrency checks.
        settings:
          enable:
            - globalstate
          disable:
            - lockorder
          enable-checks:
            - contextpolicy/test-context
          disable-checks:
            - errorownership/text-classification
```

The plugin runs gohawk's conservative default set;
opt-in analyzers and checks are suppressed unless explicitly enabled. Its
settings accept `enable` and `disable` analyzer lists, `enable-checks` and
`disable-checks` lists of stable check IDs, or `enable-all: true` to start with
every analyzer and check. Explicitly enabling a check also enables its owning
analyzer, unless that analyzer is explicitly disabled. Run `gohawk list` and
`gohawk list -checks` to see valid names and check IDs.

See [Configuration](../configuration/) for opt-in checks and analyzer-selection
behavior.

## Run it

Use the generated binary in place of the standard golangci-lint executable:

```sh
./custom-gcl run
```

Rebuild the custom binary after changing the gohawk or golangci-lint version in
`.custom-gcl.yml`.
