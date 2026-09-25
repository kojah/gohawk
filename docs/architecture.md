---
title: Codebase layout
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

Groups follow the problem being diagnosed: **Concurrency and synchronization**
(`concurrency`), **Resources and lifecycle** (`resources`), and **General
correctness** (`correctness`). Source packages and documentation use the same
grouping. Within each group, the catalog automatically sorts analyzers by name
for CLI and website presentation; their stable execution order is separate.

## Tests and examples

Each analyzer keeps its test programs in a local `testdata` directory. A
`// want "message"` comment marks code that should produce a diagnostic. Code
without that marker is expected to be accepted.

Documentation examples come from those same test programs. `go generate ./...`
updates catalog and reference content quickly while leaving the existing
examples in place. After changing example fixtures or analyzer behavior, run
`make generate-examples` to validate them with the real analyzers and update
their generated blocks. CI runs `make generated-check` with live example
validation, so stale committed examples fail the build.

Larger analyzers use shared control-flow and data-flow tools to decide whether
a diagnostic is safe to report. The shared engine, its layering, and the rules
the architecture tests enforce are described in the
[development architecture notes](https://github.com/kojah/gohawk/blob/main/docs/development/architecture.md).

## How a lifecycle analyzer is shaped

Ownership and lifecycle analyzers all share one shape, and it flips the usual
burden of proof: before the analyzer may complain that nothing cleans a value
up, it first has to show that something promised to.

1. An **obligation finder** works out what a worker or callee promises — a
   channel it signals, a group it settles, a cancel it must release — and ties
   that back to exact values in the caller.
2. A **classifier** labels each later instruction once as `join`, `transfer`,
   `unknown`, or `none`. `unknown` is anything the analysis cannot see
   through; it hides the diagnostic rather than counting as a weaker join.
3. A single **flow query** decides the outcome: honored when exact actions
   cover every return, unknown when only opaque ones do, and violated
   otherwise. `ssaflow.EvaluateObligation` is that query; the analyzer
   supplies only its labels, and an opaque handoff on one path never excuses
   an unrelated early return.

So a default diagnostic needs real evidence of both a promise and a broken
one. New patterns are almost always new classifier rules; the flow query
itself does not change.

## How a run is driven

`gohawk ./...` does not load a whole program into one process. It runs the
analyzers through `go vet -vettool=<gohawk> -json`, so the `go` command drives
the analysis one package at a time: dependencies are type-checked from export
data, each package's SSA is built and freed before the next, and results and
facts are cached. That keeps memory bounded on projects with large
dependencies, where loading the entire closure at once would exhaust it, and it
makes a rerun after an edit re-analyze only the changed package and its
importers. gohawk then post-processes go vet's JSON to produce its rich output,
or pass the JSON through, and it validates selection and
analyzer flags up front so a bad name fails once.

The same binary is the tool go vet invokes: when go vet runs it with a unit
configuration file, the unitchecker driver analyzes that one package in
process. So the standalone command and the vet-tool invocation are the same
program in its two roles.

## Where to start

For a compact concurrency analyzer, start with
`internal/analyzers/concurrency/channelsafety`. For a lifecycle analyzer that follows
program flow, start with `internal/analyzers/resources/deferinloop`.

Continue with [How to contribute](../contributing/) for the steps involved in
adding or changing an analyzer.
