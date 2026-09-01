---
title: Understanding the codebase
description: How gohawk's command, catalog, analyzers, tests, and documentation fit together.
sidebar:
  order: 2
---

gohawk separates its public analyzer catalog from the command that runs it and
from the individual analyzer implementations. Most contributions touch one
analyzer package, its fixtures, and its documentation rather than every layer.

## Request flow

```text
main.go
  → internal/cli
  → analyzers
  → internal/analyzers/<group>/<analyzer>
  → internal/passes and internal/ssaflow
```

- `main.go` is a thin executable entry point.
- `internal/cli` parses selection flags, resolves an execution plan, and hands
  the selected analyzers to Go's analysis driver.
- `analyzers` is the public catalog. It defines groups, opt-in status, stable
  execution order, and the metadata used by the CLI and documentation.
- `internal/analyzers/<group>/<analyzer>` gives each analyzer an independent
  package. Contracts, ownership, reliability, and testing mirror catalog
  metadata as organizational containers rather than Go package boundaries.
- `internal/catalog` contains the validated internal analyzer registry.
- `internal/check` contains stable check identities, diagnostic reporting, and
  suppression policy. `internal/flagvalue` contains shared analyzer flag
  parsers, while `internal/trace` provides cross-cutting evidence tracing.
- `internal/passes` contains prerequisite analysis passes shared by
  otherwise independent analyzers, including cross-package lifecycle facts.
- `internal/syntax` contains syntax and type helpers.
  `internal/ssaflow` contains control-flow, call, value, and ownership
  helpers for SSA-backed analyzers.
- `tools` contains repository-development commands such as documentation
  generation and dogfood benchmark measurement; these are not shipped as part
  of the gohawk application.

The dependency direction is deliberate: implementations depend on shared
analysis helpers, and the public catalog depends on implementations. Shared
helpers never import the catalog or an analyzer package.

## Precision infrastructure

SSA value relationships are deliberately separate. Exact identity follows
conversions and local load/store pairs; derivation follows data flow;
containment represents aggregates; and escape helpers model ownership transfer.
An analyzer must choose the narrowest relationship that proves its contract.

Lifecycle analyzers share an internal prerequisite that computes memoized
all-return-path summaries and exports compact facts for imported functions.
This lets a helper's behavior cross package boundaries without guessing from
names such as `CloseThing` or `WaitFor`. Third-party APIs whose bodies are not
available are matched in the centralized library-contract registry.

Focused fixtures guard individual proofs. The pinned, human-reviewed cohorts
under `benchmarks/precision` additionally guard repository-scale behavior:
reviewed false positives must stay absent and reviewed true positives must stay
present.

## Analyzer declaration

Each analyzer has three connected pieces:

1. Its package exports `Analyzer`, which constructs the `analysis.Analyzer`.
2. `analyzers/catalog_specs.go` declares its group, activation, checks, check
   activation, and suggested-fix support.
3. `analyzers/analyzers.go` places it in the stable execution order.

`catalog.NewCatalog` validates these declarations at construction time.
It rejects missing checks, duplicate identities, and incomplete execution
order rather than allowing catalog drift.

## Tests and documentation

Each analyzer package runs against its local GOPATH root under
`internal/analyzers/<group>/<analyzer>/testdata/src`. A `// want "message"` comment
marks a diagnostic that must be reported; unmarked code is an accepted form
that must remain quiet.

Documentation examples live in the same fixture packages between
`//gohawk:example` markers. `go generate ./...` runs the documentation
generator, which executes the real analyzers and writes their actual
diagnostics and source ranges into the Markdown examples and website manifest.
The examples are therefore test-backed rather than separately maintained
pseudocode.

## Where to start

For a syntax-based analyzer, begin with
`internal/analyzers/ownership/deferinloop/analyzer.go`. For a small SSA-backed
analyzer, begin with `internal/analyzers/ownership/exitpolicy/analyzer.go`. The goroutine,
resource-lifetime, closed-domain, and lock-order analyzers model substantially
more control and data flow and are better approached after reading the shared
SSA helpers.

Continue with [How to contribute](../contributing/) for the complete sequence
for adding or changing an analyzer.
