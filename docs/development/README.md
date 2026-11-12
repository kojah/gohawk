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

## Editors

- [Editor integration](editor-integration.md) — the `gohawk lsp` server and
  the VS Code extension: scheduling, what is never shown as clean, and how
  the acceptance run measures latency and memory.

## Models

- [Techniques and related work](techniques.md) — the analysis techniques
  gohawk combines, where each lives in the code, and what it borrows from
  Infer, Pulse, and the literature.
- [SSA in gohawk](ssa-in-gohawk.md) — how the analyzers read SSA, and every
  SSA form they handle.
- [Architecture in detail](architecture.md) — the shared engine, how a run is
  driven, and the invariants the architecture tests enforce.
- [SSA responsibility layers and file limit](ssaflow-layer-split.md) — the
  value, CFG, call and path-proof boundaries and the repository-wide limit of
  twenty direct production files per directory.
- [Codebase cleanliness review](cleanliness-review-2026-10-06.md) — reviewed
  boundaries, completed cleanup and remaining source-review questions.
- [Fact model](fact-model.md) — modular function summaries, their
  publication, and the guarantees each component can establish.
- [Points-to model](points-to-model.md) — the per-function region graph that
  answers identity, containment, and storage questions.
- [Storage model](storage-model.md) — observation-time storage evidence
  shared by the analyzers.
- [Shared value-walk review](value-walk-review.md) — consolidated traversal
  mechanics and the distinct state, ordering and cycle policies retained.
- [Storage and summary decision review](storage-summary-review.md) — shared
  projection and capture mechanics, with domain proof boundaries preserved.
- [Preconditions](preconditions.md) — what a summarized function requires of
  the objects it is handed, and how a caller's state is checked against it.

## Working on a change

The analyzer index and per-package API references are generated Markdown and
are ignored by Git. Run `make generated-markdown` to recreate them without
changing tracked pages, or `make generate` to refresh all generated content.
CI and site commands recreate ignored pages before checking or building;
pages containing handwritten prose and generated sections remain tracked.

- [Performance workflow](performance-workflow.md) — reuse caches and keep
  disposable Go writes off the shared disk while profiling and validating.
- [Debugging reference](debugging-reference.md) — the SSA, fact, and
  evidence-trace dumps, and how to read them.
- [Stress suite](stress-suite.md) — the release-only memory-limited runs on
  pathological code and the analyzer fuzz target.
- [Decisions](decisions/) — why a policy exists, one file per decision.

## Measurements and history

- [Consolidation completion audit](consolidation-completion-audit.md) — current
  evidence, reviewed duplicate candidates and remaining verification gaps.
- [Heap evidence loss](heap-evidence-loss.md) — pinned measurements of where
  cached heap evidence is lost.
- [Analysis performance](analysis-performance-2026-10-06.md) — cold and cached
  driver costs, analyzer timing, and heap-state snapshot measurements.
- [Resource test performance](resource-test-performance.md) — baseline
  measurements of resource analyzer test runtime.
- [Synchronization graph](synchronization-graph.md) — why the cross-goroutine
  event graph was retired, and what to reuse if the idea returns.
- [Handoffs](handoffs/) — session handoff notes that record the state of a
  long piece of work.
- [Drafts](drafts/) — unpublished writing, such as blog drafts.
