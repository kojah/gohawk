---
title: Understanding the codebase
description: A short guide to how gohawk is organized.
sidebar:
  order: 2
---

Most changes to gohawk are local. Adding or changing a check usually means
working in one analyzer package, its test cases, and its documentation.

## How a run works

```text
command → selected analyzers → shared analysis tools → diagnostics
```

The main parts are:

- `main.go` and `internal/cli` handle the command line and decide which
  analyzers to run.
- `analyzers` is the public list of analyzers. It records their names, groups,
  checks, and whether they run by default.
- `internal/analyzers/<group>/<analyzer>` contains the analyzer
  implementations. Each analyzer has its own package.
- `internal/check` reports diagnostics and handles ignore comments.
- `internal/passes`, `internal/syntax`, and `internal/ssaflow` provide analysis
  tools used by more than one analyzer.
- `tools` contains development commands. It is not part of the shipped
  application.

Dependencies point in one direction: analyzers may use shared tools, but the
shared tools do not depend on individual analyzers or the public catalog.

## Where analyzers are registered

An analyzer appears in three places:

1. Its package exports an `Analyzer` value.
2. `analyzers/catalog_specs.go` describes how users can select it.
3. `analyzers/analyzers.go` places it in a stable running order.

The catalog checks these declarations when it is created, so missing or
duplicate entries fail early.

## Tests and examples

Each analyzer keeps its test programs in a local `testdata` directory. A
`// want "message"` comment marks code that should produce a diagnostic. Code
without that marker is expected to be accepted.

Documentation examples come from those same test programs. Running
`go generate ./...` checks the examples with the real analyzers and updates
the generated documentation. This keeps examples and behavior in sync.

Larger analyzers use shared control-flow and data-flow tools to decide whether
a diagnostic is safe to report. Those details are intentionally outside this
overview.

## Where to start

For a small syntax-based analyzer, start with
`internal/analyzers/ownership/deferinloop`. For a small analyzer that follows
program flow, start with `internal/analyzers/ownership/exitpolicy`.

Continue with [How to contribute](../contributing/) for the steps involved in
adding or changing an analyzer.
