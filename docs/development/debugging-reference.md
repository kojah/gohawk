---
title: Debugging reference
description: The SSA, fact, and evidence-trace dumps, and how to read them.
sidebar:
  order: 3
---

The analyzers reason over SSA, and the mapping from Go source to SSA is not
obvious: `defer` lowers to `*ssa.Defer`, closures become `*ssa.MakeClosure`
with explicit bindings, loops and `&&`/`||` introduce phi nodes, and method
calls acquire interface wrappers. Do not reconstruct that mapping by hand.
Dump it, then reason about what the analyzer actually sees.

## SSA dump

```text
gohawk ssa [-func NAME] [-tests] package...
```

Prints the SSA of the matching functions in the given packages. Use `-func`
to narrow to one function and `-tests` to include test files. This is the
first thing to run when a classifier label is surprising.

## Fact dump

```text
gohawk facts [-func NAME] [-tests] package...
```

Prints the exported lifecycle summaries for the given packages, decoded per
parameter. Only summarized functions appear: a function that is missing has
no fact and is `unknown` to every consumer, which is different from a
function whose fact shows a clear bit. See
[the fact model](../fact-model/).

## Evidence trace

```text
gohawk -gohawk-trace=ANALYZER[,CHECK,...] package...
gohawk -gohawk-trace=all package...
```

Emits one JSONL event per evidence decision to stderr. The flags below are
regenerated from the flag set the analyzers register, by `go generate ./...`;
do not edit them by hand.

<!-- gohawk:generated-trace-flags:start -->
| flag | effect |
|---|---|
| `-gohawk-trace` | emit JSONL evidence for comma-separated analyzers/checks, or all |
| `-gohawk-trace-file` | append trace JSONL to this file instead of stderr |
| `-gohawk-trace-function` | limit evidence tracing to functions containing this text |
| `-gohawk-trace-source` | limit evidence tracing to positions containing this path[:line] |
<!-- gohawk:generated-trace-flags:end -->

The flags carry a `gohawk-` prefix because `x/tools` owns the generic `-trace`
flag. Enabling tracing must never change which diagnostics are reported.

### Phases

| phase | meaning |
|---|---|
| `candidate` | a construct the analyzer might report — the obligation it found |
| `evidence` | a fact for or against reporting it — typically one classifier label |
| `decision` | the outcome: reported, suppressed by an ignore comment, removed by check selection, or unknown |
| `fix` | a suggested edit was offered or rejected |

Every event names its analyzer and check and carries a stable kebab-case
reason code. Events include the SSA text they concern, so a trace for one
candidate reads as an annotated SSA walk.

### Reading one candidate

1. Start at its `candidate` event: that is the obligation and its exact SSA
   value.
2. Follow its `evidence` events in order. Each consumption of the tracked
   value appears once with the label the classifier gave it. An `unknown`
   label ends the proof conservatively.
3. Read the `decision` and its reason code.

A `decision` of unknown means some consumption was opaque and the analyzer
declined to report. That is the design working, not a defect, unless the
opaque consumption is a shape the classifier ought to recognize.

## Incremental analysis

`gohawk ./...` reloads and re-analyzes the whole program on every run. For a
change-and-rerun loop, run it as a `go vet` tool instead:

```text
go vet -vettool=$(which gohawk) ./...
```

The `go` command then caches each package's analysis and facts, so a rerun
after an edit re-analyzes only the changed package and its importers. Output
uses `go vet`'s format rather than gohawk's rich rendering.
