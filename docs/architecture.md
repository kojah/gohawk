---
title: Architecture
description: How gohawk's command, catalog, analyzers, tests, and documentation fit together.
sidebar:
  order: 2
---

gohawk separates its public analyzer catalog from the command that runs it and
from the individual analyzer implementations. Most contributions touch one
analyzer group, its fixtures, and its documentation rather than every layer.

## Request flow

```text
main.go
  → internal/cli
  → analyzers
  → internal/analyzers/<group>
  → analysisutil and analysisutil/ssa
```

- `main.go` is a thin executable entry point.
- `internal/cli` parses selection flags, resolves an execution plan, and hands
  the selected analyzers to Go's analysis driver.
- `analyzers` is the public catalog. It defines groups, profiles, tags, stable
  execution order, and the metadata used by the CLI and documentation.
- `internal/analyzers` contains the analyzer implementations, grouped by
  contracts, ownership, reliability, and testing.
- `internal/analyzerbase` contains the internal catalog model, stable check
  identities, diagnostic helpers, and shared flag value types.
- `analysisutil` contains syntax and type helpers. `analysisutil/ssa` contains
  control-flow, call, value, and ownership helpers for SSA-backed analyzers.

The dependency direction is deliberate: implementations depend on shared
analysis helpers, and the public catalog depends on implementations. Shared
helpers never import the catalog or an analyzer group.

## Analyzer declaration

Each analyzer has three connected pieces:

1. Its implementation file defines an `analysis.Analyzer` and run function.
2. Its group's `analyzers.go` declares an `analyzerbase.AnalyzerSpec` with the
   analyzer profile, checks, check profiles, tags, and suggested-fix support.
3. `analyzers/analyzers.go` places the analyzer in the stable execution order.

`analyzerbase.NewCatalog` validates these declarations at construction time.
It rejects missing checks, duplicate identities, unknown tags, invalid
profiles, and incomplete execution order rather than allowing catalog drift.

## Tests and documentation

`analyzers/analyzers_test.go` runs analyzers against packages under
`testdata/src`. A `// want "message"` comment marks a diagnostic that must be
reported; unmarked code is an accepted form that must remain quiet.

Documentation examples live in the same fixture packages between
`//gohawk:example` markers. `go generate ./...` runs the documentation
generator, which executes the real analyzers and writes their actual
diagnostics and source ranges into the Markdown examples and website manifest.
The examples are therefore test-backed rather than separately maintained
pseudocode.

## Where to start

For a syntax-based analyzer, begin with
`internal/analyzers/ownership/deferinloop.go`. For a small SSA-backed analyzer, begin with
`internal/analyzers/ownership/exitpolicy.go`. The goroutine, resource-lifetime,
closed-domain, and lock-order analyzers model substantially more control and
data flow and are better approached after reading the shared SSA helpers.

Continue with [How to contribute](../contributing/) for the complete sequence
for adding or changing an analyzer.
