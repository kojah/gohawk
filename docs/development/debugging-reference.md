---
title: Debugging reference
description: The SSA, fact, and evidence-trace dumps, and how to read them.
sidebar:
  order: 4
---

The analyzers reason over SSA, and the mapping from Go source to SSA is not
obvious: `defer` lowers to `*ssa.Defer`, closures become `*ssa.MakeClosure`
with explicit bindings, loops and `&&`/`||` introduce phi nodes, and method
calls acquire interface wrappers. Do not reconstruct that mapping by hand.
Dump it, then reason about what the analyzer actually sees.

## SSA dump

```text
gohawk ssa [-func NAME] [-tests] [-regions] package...
```

Prints the SSA of the matching functions in the given packages. Use `-func`
to narrow to one function and `-tests` to include test files. This is the
first thing to run when a classifier label is surprising.

## Fact dump

```text
gohawk facts [-func NAME] [-tests] [-regions] package...
```

Prints the exported lifecycle summaries for the given packages, decoded per
parameter, each with its heap projection as `heap …` lines, including the
`requires` lines that name the methods it calls on what it was handed. Only summarized
functions appear: a function that is missing has no fact and is `unknown`
to every consumer, which is different from a function whose fact shows a
clear bit. See [Inferred facts](../fact-model/).

`-regions` prints every function of the package, private helpers and
literals included, with the summary the registry holds for it and the
points-to graph the analysis built: each value's pointees, an `applied`
line for every call a callee summary was substituted at, an `unsummarized`
line with its reason (`no-summary`, `closure-callee`, `interface-call`,
`dynamic-call`, `started`) for every call the graph forgot through instead,
a `widened` line
for every slot whose pointees outgrew the bound and became unknown, an
`escaped` line naming the first instruction that escaped each slot in each
way, and the disjointness answers given. A claim that looks wrong is read backwards from
here: the `heap effect` or `heap edge` behind it, then the `escaped` or
`applied` line that produced it, then the callee's own section.

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
| `-gohawk-timing-file` | append one JSONL record per analyzer and package with wall time and allocation to this file |
| `-gohawk-trace` | emit JSONL evidence for comma-separated analyzers/checks, or all |
| `-gohawk-trace-candidate` | limit evidence tracing to the proof of candidates whose position contains this path[:line] |
| `-gohawk-trace-file` | append trace JSONL to this file instead of stderr |
<!-- gohawk:generated-trace-flags:end -->

The flags carry a `gohawk-` prefix because `x/tools` owns the generic `-trace`
flag. Enabling tracing must never change which diagnostics are reported.

### Timing

`-gohawk-timing-file=PATH` records cost rather than evidence: one JSON line
per analyzer per package with the wall time and the bytes allocated while
that analyzer ran. Under `go vet` each package is analyzed in its own
process and the records are appended, so one file covers a whole module.
Summing by analyzer shows which proof dominates a run, and sorting by package
shows where a large dependency graph spends its time. Timing is measured
around the analyzer's `Run` only; building SSA and loading types are the
driver's cost and appear in the process time instead.

### Phases

| phase | meaning |
|---|---|
| `candidate` | a construct the analyzer might report — the obligation it found |
| `evidence` | a fact for or against reporting it — typically one classifier label |
| `considered` | a proof step that was evaluated and did not hold |
| `decision` | the outcome: reported, suppressed by an ignore comment, removed by check selection, or unknown |
| `fix` | a suggested edit was offered or rejected |

Every event names its analyzer, its check, the candidate whose proof it
serves, and a stable kebab-case reason code. The candidate is what
`-gohawk-trace-candidate` selects on, so one proof can be read whole even
where its steps run inside a callee body in another file. Events include the
SSA text they concern, so a trace for one candidate reads as an annotated SSA
walk.

### Reading one candidate

1. Start at its `candidate` event: that is the obligation and its exact SSA
   value.
2. Follow its `evidence` events in order. Each consumption of the tracked
   value appears once with the label the classifier gave it. An `unknown`
   label ends the proof conservatively.
3. Read its `considered` events. Each names a suppression that was tried and
   did not hold, so a suppression you expected is either absent, meaning the
   proof never reached it, or present, meaning its rule did not match.
4. Read the `decision` and its reason code.

A `decision` of unknown means some consumption was opaque and the analyzer
declined to report. That is the design working, not a defect, unless the
opaque consumption is a shape the classifier ought to recognize.

### Synchronization summary cutoffs

The channel-dependency and mixed lock-dependency checks emit `protocol-cutoff`
evidence before their final decision when summary inference retains a cutoff.
Its position identifies the rejecting instruction when SSA supplies one,
otherwise the containing function. Details include the instruction kind and
text, the function, the final summary reason, and a control-flow shape category.
For helper failures, `caller-0` is the innermost call site, followed by its
callers; at most eight sites are retained and `chain-truncated` names an omitted
outer suffix. Cached helpers retain the same leaf attribution without
rerunning inference or modifying another caller's chain.

This is the first cutoff of the final inference attempt, not a list of every
unsupported operation or evidence of a missed bug. Alternative collectors can
retry an initial rejection; only the final attempt is attributed. `unspecified`
means a function-level boundary has no retained rejecting instruction. Missing
imported facts and binding failures may identify the caller rather than a
dependency body that is unavailable. No source positions or cutoff metadata
are exported in cross-package facts.

### Give-up events from the shared engine

The shared proofs in `internal/ssaflow` never call the tracer, but they report
where they stopped through the search budget that scopes each query. When an
analyzer attaches its probe to a budget, every give-up inside that query
appears as an `evidence` event with outcome `unknown`, attributed to the same
candidate, carrying a specific reason and the instruction that blocked it:

| reason family | examples | what to look at |
|---|---|---|
| storage | `storage-address-escapes`, `storage-conflicting-writes`, `storage-write-after-observation`, `storage-not-local` | the named store, call, or merge; the cell was not proved to hold one value there |
| alias | `disjoint-paths`, `disjoint-objects`, `unescaped-local`, `shared-slot`, `unknown-pointee`, `structural-walk` | the points-to graph's answer to a may-alias question; the first three are disjointness claims. `gohawk ssa -regions` prints each value's pointees, named by kind and origin, with entries carried around a back edge marked stale |
| summary | `summary-body-unavailable`, `summary-recursive` | the named callee; its body could not be summarized, so effects cannot be ruled out |
| completion | `evidence-not-found`, `evidence-unavailable` at a launch site | the callee resolved from that launch never covered the target with the method sought |
| budget | `budget-exhausted` | the query that spent the last unit; a cut answer is not a decision |

A must-proof over the graph, such as nilargument's nil slot, can instead be
wrong because a call before it was not summarized. When nilargument reports,
its trace lists each earlier call that can reach the judged call and was
unsummarized, as `earlier-call-unsummarized` evidence with the same reason
codes as the dump, and counts them in `earlier-calls`. A `no-summary` on a
callee whose facts exist means the summary was not registered when the
caller's graph was built.

These events say why evidence ran out, never what was decided, so the analyzer
decision that follows them is still the one to read. A budget with no probe
attached stays silent, and a disabled probe attaches nothing, so give-up
reporting costs nothing unless a trace is on.

## Incremental analysis

`gohawk ./...` already runs the analyzers through `go vet` under the hood, so
the `go` command type-checks dependencies from export data and caches each
package's analysis and facts. A rerun after an edit re-analyzes only the
changed package and its importers; there is no whole-program reload to avoid.

Running `go vet -vettool=$(which gohawk) ./...` directly is equivalent for the
analysis and is what a build system that already integrates `go vet` would use;
the only difference is that go vet renders the diagnostics in its own terse
format instead of gohawk's.
