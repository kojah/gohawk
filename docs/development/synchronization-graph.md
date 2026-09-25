---
title: Synchronization graph (retired)
description: Why the cross-goroutine event graph and its cycle checks were removed, and what to reuse if the idea returns.
sidebar:
  order: 7
---

The synchronization graph (`internal/syncmodel`) turned complete concurrency
summaries into bounded event-order fragments for one parent and its children,
and four experimental checks proved deadlock cycles over it:
`lockorder/lock-and-join`, `lockorder/channel-lock-cycle`,
`lockorder/waitgroup-lock-cycle`, and `channelsafety/dependency-cycle`. The
graph, the four checks, and their fixtures were removed on 2026-09-25. The last
revision containing them is `c69764505b72b1bc6400cc55ec01c3e64b12d36c`;
restore a file with `git show c697645:internal/syncmodel/graph.go`.

## Why it was retired

None of the four checks ever reported a finding:

- The frozen [1,000-repository audit](../../benchmarks/precision/audits/overnight-2026-09-24-1000.md)
  ran them with every check enabled and test files included.
- The targeted [cycle-check recall audit](../../benchmarks/precision/audits/cycles-2026-09-24.md)
  traced 100 repositories chosen for their concurrency findings. Most
  candidates stopped at an incomplete summary, first blocked by a loop or an
  unsupported branch (76) or by a call with no usable summary (91). The 13
  cycle decisions that were reached were all correct rejections.
- The [deadlock-fix prevalence study](../../benchmarks/precision/audits/deadlock-fixes-2026-09-24.md)
  ran the checks on the parent revision of 45 real fixed Go deadlocks. None had
  one of the four shapes.

The checks proved a deadlock on every path through fresh local resources. That
shape fails on its first run and rarely ships. Shipped deadlocks are
conditional: a partner that is missing on an error, shutdown, or configuration
path. The study's recommendation became `producerlifecycle/stopped-loop-send`
and `producerlifecycle/unclosed-range`, which need ordered summaries but no
cross-goroutine graph.

The broader lesson is about absence claims. An unavoidable cycle says that no
participant can ever unblock a wait, so every participant and every operation
must be known. One opaque call anywhere in the graph made the whole proof
unknown.

## What remains

- `internal/passes/concurrencyfacts` still composes complete, bounded ordered
  effects per function and binds them to caller values. `producerlifecycle`,
  `goroutineownership`, `concurrentcapture`, `lockorder`, and `channelsafety`
  consume its linear summaries.
- The mutex region fold that `concurrentcapture` used moved into that
  analyzer as `lockRegion`.

## Feasibility policy worth reusing

The hardest part of the graph to rebuild is deciding when the branch choices
an alternative assumes can hold in one execution. A future check that reports
a wait blocked only on an error path needs the same decision. The retired
`feasibility.go` used this policy:

- Condition identity and stability come from `ssaflow.GuardCondition`, the
  guards lockorder and resourcelifetime prune with. A condition stored apart
  from its instruction also needed `GuardComparison`, which was removed with
  the graph and can be restored from the same revision.
- Taking both arms of a stable guard (a parameter, a constant, or a value
  computed once) is a contradiction. Taking both arms of a guard read from
  memory is only unknown, because an unseen store could explain it.
- One value required to equal two different constants is a contradiction.
- Conditions on distinct root parameters, or on results of calls to distinct
  functions, vary independently. Two results of one function may be equal.
- A condition read from memory, computed from other values, without an
  identity, or bound from a callee may correlate with any other condition, so
  it is feasible only as the sole condition. Anything else is unknown, never
  assumed feasible.
