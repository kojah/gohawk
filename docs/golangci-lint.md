---
title: golangci-lint integration
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
    version: main
```

The plugin is currently available from `main`. Replace that value with a
released gohawk version once a release containing the plugin is available.

Build the custom binary with the Go 1.26 toolchain:

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
            - contextpolicy
```

The plugin runs gohawk's conservative default analyzer profile. Its settings
accept `enable` and `disable` lists, or `enable-all: true` to start with the
complete analyzer set. Run `gohawk list` to see valid analyzer names.

See [Configuring gohawk](../configuration/) for profile and analyzer-selection
behavior.

## Run it

Use the generated binary in place of the standard golangci-lint executable:

```sh
./custom-gcl run
```

Rebuild the custom binary after changing the gohawk or golangci-lint version in
`.custom-gcl.yml`.
