# Production queue after the conditional-copy repair

Beads `gohawk-dho.4.9` reconciles all 55 original production sites after
`116376a8`. The [site ledger](final-production-fp-refresh-2026-10-03.tsv)
preserves every original repository, revision, analyzer, position, check and
verdict. Historical ledgers are unchanged. This is the fixed production queue,
not the full precision-regression corpus or a new label review of every batch.

## Execution and results

The immutable `.build/goal-conditional-copy-reviewed` executable implements
`116376a8`, with SHA-256
`f526db1dec9897b3846ea7f92c7ed58e499359944eda27179e1ed46d21900c7f`.
Twenty-nine package scopes across twenty-nine repositories cover all 55 sites.
Each checkout matched its original pin and was clean before scanning. Every
scope terminates with exit zero and empty stderr. Commands use
`go vet -vettool=<binary> -enable-all -json`, selected check tracing,
`CGO_ENABLED=0`, `GOWORK=off` and `GOFLAGS=-mod=readonly`, with two concurrent
scans. Candidate tests, generators and applications are not executed.

| Original sites | Count |
| --- | ---: |
| Absent in successful package scans | 52 |
| Assessed goiardi FP targets still reported | 2 |
| Original Promu review corrected to a true positive, still reported | 1 |
| Total | 55 |

The two goiardi sites remain `shovey/sql_funcs.go:460:13` and `492:13`.
Promu retains `cmd/release.go:191:14`. Each has multiplicity one at its exact
original source/check key; none moves to another column. Urunc's original
`internal/metrics/metrics.go:61:16` finding is absent after the shared heap-copy
repair. Comparing the complete diagnostic multisets for all twenty-nine package
scopes against the e395 refresh finds exactly that one removal and no additions.
No other package finding changes.

The 52 absences include three unresolved larger-model sites: Openase's two
transport sites and Ferro's process site. Their silence receives no correction
credit. Twelve target traces still contain cutoff events, at the same original
sites as the prior refresh. A cutoff count is not a claim that each event
decides the final result. Historical structural correction/policy receipts are
kept separate from this current observation; silence alone proves no cleanup.

Complete JSON, traces, clean-pin checks, terminal receipts and full diagnostic
comparison are in `.build/goal-final-queue-116376/`. Reconciliation is in
`site-results.json`; `diagnostic-comparison.json` checks complete package output.
The TSV records target decisions, cutoffs and receipt paths per original key.

## Separate Rune control

At Rune pin `3e2165f8983280542c985947378dfa740a397d03`, the same executable
scans `./internal/ide/idepkg ./internal/workspace` with `CGO_ENABLED=1`.
It terminates with exit zero, empty stderr and zero complete diagnostics.
Both original control keys remain absent. This is the thirtieth terminal scope
and is excluded from the 55-site tally. The earlier incorrect workspace package
path remains a historical load error, not validation evidence.

## Current implementation scope

The subsequent [numeric family migration](resource-family-enum-2026-10-03.md)
in `da87150d` preserves the policy bodies. Its four affected pinned controls
retain all ten diagnostics and 65,436 trace records exactly. This is separate
affected-change evidence; the 55-site executable above is not relabeled as a
binary from da87150d. Current source inventory still covers the same 359
production paths, now with 2,290 functions. The whole-body and partial-block
candidate scans retain 55 and 33 groups respectively, with no group added or
removed after either follow-up. Existing source-backed dispositions remain
necessary; scan counts alone do not prove semantic consolidation.

Fresh current-source local probes reproduce all twelve Ferro and three goiardi
assessment rows exactly, with no cutoff:
`.build/goal-final-ferro-116376-probe/results.json` and
`.build/goal-final-goiardi-116376-probe/results.json`. They distinguish implicit
zero storage, scalar result publication and receiver-conditioned feasibility
from mutable-global OR predicates versus fixed caller snapshots. Openase's
pinned Session.Close still delegates channel close while Wait consumes exit and
copy completion. Target resolution alone does not supply these missing models.
The [earlier source assessment](execution-adapter-consolidation-2026-10-03.md)
retains the precise contracts and limits. No budget increase, name exemption,
check retirement or new completion guarantee is credited to this refresh.

Both implementation follow-ups pass their focused controls, canonical
`make verify` gates and final architecture checks. This ledger refresh changes
only audit/development documentation and reuses those unaffected code receipts.
The overall requirement closure remains separate from fixed-queue completion.
