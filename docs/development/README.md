# Development reference

These pages are the maintained reference for working on gohawk's analyzers.
They are read in the repository and are not published on the documentation
site, which keeps its pages to what a user can rely on. Detail belongs here:
precision boundaries, proof mechanics, measurements, and the reasons behind a
rule. The public pages link here when a reader wants the full story.

Start with [Understanding SSA](../understanding-ssa.md) and the public
[architecture overview](../architecture.md) if you are new to the codebase.

## Analyzers

- [analyzers/](analyzers/) — one note per analyzer: every accepted and
  reported boundary, why it holds, the fixtures that pin it, and known gaps.

## Models

- [Techniques and related work](techniques.md) — the analysis techniques
  gohawk combines, where each lives in the code, and what it borrows from
  Infer, Pulse, and the literature.
- [Architecture in detail](architecture.md) — the shared engine, how a run is
  driven, and the invariants the architecture tests enforce.
- [Fact model](fact-model.md) — modular function summaries, their
  publication, and the guarantees each component can establish.
- [Points-to model](points-to-model.md) — the per-function region graph that
  answers identity, containment, and storage questions.
- [Storage model](storage-model.md) — observation-time storage evidence
  shared by the analyzers.
- [Preconditions](preconditions.md) — what a summarized function requires of
  the objects it is handed, and how a caller's state is checked against it.

## Working on a change

- [Debugging reference](debugging-reference.md) — the SSA, fact, and
  evidence-trace dumps, and how to read them.
- [Decisions](decisions/) — why a policy exists, one file per decision.

## Measurements and history

- [Heap evidence loss](heap-evidence-loss.md) — pinned measurements of where
  cached heap evidence is lost.
- [Resource test performance](resource-test-performance.md) — baseline
  measurements of resource analyzer test runtime.
- [Synchronization graph](synchronization-graph.md) — why the cross-goroutine
  event graph was retired, and what to reuse if the idea returns.
- [Handoffs](handoffs/) — session handoff notes that record the state of a
  long piece of work.
- [Drafts](drafts/) — unpublished writing, such as blog drafts.
