# Program-entry context follow-up

Beads `gohawk-dho.37` reassesses the k8ssandra context finding from the
[remaining-family assessment](remaining-fp-assessment-2026-10-01.md).
The original finding is absent in a successful corrected scope; both reviewed
Openase cancellation true positives remain. Seven of the original 22 replayed
production sites now have scoped corrections, leaving 15 unresolved locations.
Frozen labels, historical batch precision and earlier replay ledgers are unchanged.

## Evidence and chosen boundary

The pinned k8ssandra `main` calls `context.WithCancel` once outside a loop.
Its cancel is passed to `SetupWithManager` only in the control-plane branch;
the other branch reaches program exit after manager execution. Actual SSA is
recorded in `.build/goal-entry-context-parent.ssa.txt`: the constructor's SSA
position is `main.go:176:35`, while its diagnostic is at `main.go:176:17`.
The branch-local handoff alone cannot establish cleanup on every return.

The check is narrowed at its existing classifier: standard contexts acquired
at most once directly in the true program entry can have process lifetime.
The return receives `process-lifetime-context`/unknown; one ordinary obligation
walk produces the final `ambiguous-cancellation-use`/unknown decision. Exact
cleanup can still win. This bounds retention at the acquisition site but never
establishes cancel invocation, child cancellation or worker completion.
`ssaflow.RunsOnceInProgramEntry` supplies the existing structural proof; no new
SSA traversal, target resolver, flow engine or fact schema is introduced.

Loops, helpers, closures, methods, other-package functions named main and
package references to main remain checked. Signal registrations are excluded:
the stop function also changes signal delivery during the process lifetime.
The coverage loss is explicit: an uncanceled entry context can remain after
its work finishes, even though that acquisition cannot accumulate across
invocations. This policy does not propagate into unconditional callee facts.

Minimized `entrycontext`, `callableentry`, and `namedentry` fixtures cover these
boundaries. The focused test verifies final unknown outcomes, exact deferred
release, excluded-form diagnostics, and exactly one process-lifetime label per
return. A parent-source overlay fails the accepted context/cause forms and their
trace expectations; the corrected focused test passes (5.537 seconds).
Actual fixture SSA is retained in `.build/goal-entry-context-fixture.ssa.txt`.
The action enum now lives beside the existing label vocabulary; this mechanical
move keeps the classifier file within the repository lint limit.

## Scoped replay

Production baseline is `c8845ed`, retained byte-for-byte as
`.build/goal-entry-context-parent`, SHA-256
`735f414e60f1a599f8b2930e4a8b0239d7d9ea08f97b358353018e5558c343d7`.
The corrected binary `.build/goal-entry-context-current` was built from that
source plus the classifier/reason change, before the later mechanical enum move
and test/documentation edits; SHA-256
`5bb0068208a49eb1e38b8dfed9aa52887e0742e562c14bbc52c53f3073e5c299`.
Hashes identify the exact executables, not clean-tree Git stamps. Binaries were
not replaced during scans.

Every scan uses `-enable-all -json`, `CGO_ENABLED=0`,
`GOFLAGS=-mod=readonly`, and `GOWORK=off`. The k8ssandra scope is `.`;
Openase is `./internal/orchestrator`. Both k8ssandra scans additionally enable
cancellationownership tracing to separate JSONL files.

| Repository and pin | Baseline | Corrected |
| --- | --- | --- |
| k8ssandra/k8ssandra-operator, `2028d352ecb495de4b6e053d99d7a77b21eb5107` | Exit 3; original report present, decision `unowned-return`/rejected. | Exit 0; absent, decision `ambiguous-cancellation-use`/unknown. |
| PacificStudio/openase, `e530faf137e764337d5beaaf68af3be159eb17aa` | Exit 3; both reviewed cancellation TPs present. | Exit 3; both retained, byte-identical nonempty diagnostic JSON. |

K8ssandra stderr is empty in both scans. Openase baseline stderr contains only
Go dependency-download notices (1,471 bytes); the corrected stderr is empty.
Exit 3 denotes diagnostics, not package-load failure. The Openase TP verdicts
come from `batch-63-findings.tsv`, not from report presence alone. Their exact
positions and receipt paths are in the [follow-up TSV](program-entry-context-followup-2026-10-02.tsv).
Candidate tests, applications and generators were not run.

The first gate found missing trace-test JSON tags, classifier file size and a
documentation reference interpreted as an architecture test. These are corrected;
the focused documentation gate passes. The stable `make verify` gate passes
all local checks, including ordinary tests (3 seconds), formatting, vet, lint,
dead-code and local dogfood (`.build/goal-entry-context-stable-verify.log`).
No full precision-regression or local race run is part of this iteration.
