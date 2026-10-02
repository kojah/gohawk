# Closure choices and opaque worker participation

Beads `gohawk-dho.4.7`.

Pinned Debian dcs `567a9be49163cbf731f25bf79890f692e04d22d9`
chooses between two captured worker closures and launches the chosen function
in a dynamic loop. The producer channels belong to this opaque worker pool;
there is no proven participant count or caller join. Literal-only capture
lookup missed the possible consumers at `sourcebackend.go:433:3` and `:562:3`.

## Boundary and evidence

One reaching-value query now serves the instruction classifier and existing
other-worker census. Any matching captured binding establishes uncertain
ownership; it never establishes an exact join or a unique callee. Phi choices
are traversed through shared ReachingWalk; conversions and loads remain opaque.
Reaching visits and binding comparisons use the candidate allowance. Interrupted
capture discovery remains unknown; underlying identity searches retain their
existing independent cost boundary. No facts or participant-count policy change.

An isolated classifier-only experiment retained both findings. Extending the
existing worker census was necessary because the launch loop need not execute.
Compiled SSA controls exercise mixed choices, unrelated choices, loops, cutoff
and fresh-query recovery. Minimized fixtures retain the unrelated-consumer and
converted-callable diagnostics. Two counterfactuals fail assertions: requiring
every closure to capture the target loses mixed/loop evidence, while accepting
unrelated bindings wrongly suppresses the unrelated case.

The [ledger](closure-choice-consumers-2026-10-02.tsv) records six terminal
parent/current receipts with empty stderr. Parent receipts are reused from the
completed initial comparison; current scans use the final immutable binary.
All 126 existing goroutine fixture findings retain identical payloads. The new
fixture falls from three findings to two, removing only its accepted pool
producer. Clean pinned Debian `./internal/sourcebackend` falls from two to zero,
removing exactly the two target sites and adding none.

Final decision tracing records both target sites as `unknown` with reason
`signal-consumed-by-worker`. Separate site 394 remains budget-exhausted and
receives no correction credit. Seven recorded production FP sites plus Rune
remain unresolved; this is not a fresh corpus census or global architecture proof.

Final binary SHA-256:
`6689a62a422c2e8e5b35ffc6017d52cd49e6cac0f3ccc88520aef8d24152560f`.
Receipts: `.build/goal-closure-choices-focused.log`,
`.build/goal-closure-choices-controls-final.log`,
`.build/goal-closure-choices-mutants/results.json`,
`.build/goal-closure-choices-final/scans.json`,
`.build/goal-closure-choices-final/debian.trace`,
`.build/goal-closure-choices-verify-final.log` and
`.build/goal-closure-choices-architecture-final.log`.
Initial canonical validation found two line-length issues and a modernize
suggestion; all were corrected. Final `make verify VERIFY_TIMINGS=1` passes all
eight targets, including ordinary tests and self-dogfood. Final architecture
validation passes. No full precision replay or local race run was used.
Graph tools were unavailable; exact source and compiled SSA supply scoped evidence.
