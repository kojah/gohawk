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
a diagnostic is safe to report. The sections below describe how those tools
are layered and which rules the tests enforce; the
[development references](../development/shared-helpers/) list the tools
themselves.

## How a lifecycle analyzer is shaped

Ownership and lifecycle analyzers share one proof shape, and it reverses the
usual burden of evidence: the analyzer must prove that something was promised
before it may complain that nothing honors the promise.

1. An **obligation finder** resolves what a worker or callee promises — a
   channel it signals, a group it settles, a cancel it must release — back to
   exact SSA values in the caller.
2. A **classifier** labels every later instruction once as `join`,
   `transfer`, `unknown`, or `none`. `unknown` is any consumption the analysis
   cannot see through, and it suppresses the diagnostic rather than acting as
   a weaker join.
3. **One flow query** decides the outcome: honored when exact actions cover
   every return, unknown when only opaque actions do, violated otherwise.

A default diagnostic therefore needs positive evidence of both an obligation
and its violation. New patterns are almost always classifier rules; the flow
query does not change.

## Shared engine

- `internal/ssaflow` owns SSA mechanics: value provenance (`ReachingWalk`),
  path-sensitive state (`WalkStates`), return coverage (`UnownedReturn`),
  storage and escape checks, and symbol matching. It provides how to walk;
  it never decides whether evidence is sufficient. That policy stays beside
  each analyzer.
- `internal/passes/lifecyclefacts` exports a per-function summary of what a
  callee does to its parameters, so an analyzer can see through a call into
  another package. Consumers use `LifecycleEvidence`, which consults local
  evidence first and imported facts second. See
  [the fact model](../development/fact-model/).
- `internal/check` and `internal/trace` provide reporting and evidence
  tracing. Every diagnostic flows through `check.Report`, which is what lets
  the tracer record whether a candidate was reported, suppressed, or removed.

## Two ways to run

`gohawk ./...` is a standalone driver: it loads the whole program and
analyzes it in one process on every run, which is what gives it its rich
output and check selection. The same binary also works as a `go vet` tool
(`go vet -vettool=gohawk ./...`), where the `go` command drives it one package
at a time and caches each package's facts and diagnostics, so a rerun after
an edit re-analyzes only the changed package and its importers.

## Invariants the tests enforce

`internal/architecture` contains repository-wide conformance tests. Each one
guards a rule that the rest of this page relies on, so the architecture and
the code cannot drift apart silently.

| test | rule it enforces |
|---|---|
| `TestInternalPackagesRespectDependencyDirection` | analyzers may use shared tools; shared tools never depend on analyzers or the catalog |
| `TestAnalyzerPackageLayout` | one package per analyzer under `internal/analyzers/<group>/<name>` |
| `TestAnalyzersUseSharedReporting` | diagnostics only through `check.Report` or `check.Reportf`, never `analysis.Pass.Report` directly |
| `TestAnalyzersUseSymbolIdentity` | well-known functions matched through `syntax.Symbol`, not reconstructed from package paths and names |
| `TestProductionCodeReturnsTerminationDecisions` | no `panic`, `log.Fatal`, or `os.Exit` in analyzer or library code |
| `TestAnalyzerCommentaryCoverage` | non-obvious spans of analyzer code carry model-level rationale |
| `TestObjectFactsStayInTheirDefiningPackage` | object facts imported and exported only in the package that defines the fact type |
| `TestAnalyzersUseSharedTraversal` | value-provenance recursion — phi fan-out and visited sets — lives only in `ssaflow` |

Conventions that are not yet enforced by a test are described in the
repository's `AGENTS.md`.

## Where to start

For a small syntax-based analyzer, start with
`internal/analyzers/ownership/deferinloop`. For a small analyzer that follows
program flow, start with `internal/analyzers/ownership/exitpolicy`.

Continue with [How to contribute](../contributing/) for the steps involved in
adding or changing an analyzer.
