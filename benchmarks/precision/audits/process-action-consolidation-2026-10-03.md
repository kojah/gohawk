# Process instruction action consolidation

Beads: `gohawk-dho.23.21.12`. Parent source: `2cd024a6`.

The post-Start flow's callback repeated ownership questions for instruction
revisits. A candidate-local memo now delegates each instruction/command key to
`processOwnershipAction` once. Primary and merged commands remain distinct.
Completed actions survive later budget depletion; unknown stays conservative
inside that candidate and is never published as a complete shared summary.
A fresh candidate retains its own admission and completion queries.

The successful-return proof moves out of reporting into cohesive `flow.go`.
It retains acquisition-time receiver load resolution, successful merges,
return ownership permissions, immediate process guards, the single flow query
and final `decideProcessReturn` policy. Its result carries the resolved command
and uncovered return to presentation. Pre-Start external-store helpers move to
the existing prefix implementation. The extracted flow function exceeds the
60-line review trigger; its callbacks share one obligation and one final
witness/decision, so they remain together rather than creating another evidence
orchestration layer. No wait guarantee, fact schema, policy or budget changes.

Actual SSA constructs a deferred Wait capturing one of two command parameters.
The regression verifies the completed action after the candidate pool is
exhausted, a separate key for the second command, a fresh cut candidate and
fresh full-budget recovery. The uncached overlay compiles and fails the revisit
assertion. Receipt: `.build/goal-process-action-mutant/test.log`.
Full process package tests pass (13.074s) and lint passes.
Receipts: `.build/goal-process-actions-focused.log` and `lint.log`.

Final `make verify VERIFY_TIMINGS=1` passes all eight canonical targets.
Receipt: `.build/goal-process-actions-verify.log`.
Twelve read-only all-check parent/current scans exit zero with empty stderr.
Complete payloads agree: process fixtures (41), entry fixtures (2), pinned Ferro (1),
rev-dep (0), of-watchdog (1) and diff (1), totaling 46 diagnostics. Both reviewed
production TP controls persist. Ferro budget silence receives no FP-correction
credit. Pins, scopes, exits and hashes are in
`.build/goal-process-actions-scans/scans.json`; `comparison.json` records full
payload comparisons.

Two additional process fixture scans enable tracing with JSON output. Both
exit zero with empty stderr. Traced diagnostic payloads match untraced results.
All 153 final trace decisions agree as complete record multisets. The parent
emits 1,839 records and the current binary 1,833: six repeated evidence-unavailable
records disappear and no records are added. These occur in the ordinary and
closure-choice fixtures, confirming real flow revisits no longer repeat those
queries. Inter-process record order is not claimed.
Receipt: `.build/goal-process-actions-traces/comparison.json`.

Reviewed SHA-256:
`83be72852d57421d799ea31391783a075e98e56d7a75392dd53753af37eb4074`.
It matches the final canonical binary; these are precommit artifacts.
The normalized scanner covers 340 production files and 2,212 declarations,
retaining five full-body and 33 partial groups. Every path/name/token candidate
signature matches the parent. A recursive CallGraphMemo owns cycle-sensitive
function-summary queries, while this memo owns fixed candidate instruction
results; their cutoff/publication contracts differ. No new general memo layer
or semantic duplication absence claim is introduced.

Graph tools remain unavailable; exact source fallback was used. Final
architecture validation after the maintained documentation updates is recorded
in `.build/goal-process-actions-architecture.log`.
No full precision replay, local race run or production FP correction credit.
Five reviewed production FP sites and broader family reconciliation remain open.
