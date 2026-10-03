# Execution adapter consolidation and completion scope

Beads: `gohawk-dho.23.21.16`. Parent: `21de366c`.

Expanding normalized inventories from internal source to the repository's
production Go source finds 358 files and 2,285 declarations: six whole-body and
40 partial groups. The additional whole-body pair is public/internal catalog
tier aggregation; seven overlapping windows repeat CLI/plugin report filtering.
This is evidence of a real scope gap in the older internal-only review.

`check.FilterAnalyzerReports` now owns cloning, exact diagnostic category
filtering, all-disabled skip, typed reporting reason/phase/outcome and restoring
pass.Report on exit. Adapter selection validation and disabled-ID construction
remain local. Original Run result/error and unfiltered analyzer behavior are
preserved. Public/internal catalog records share `catalog.MostTrustedTier` via
an accessor, preserving empty experimental default and strongest selected tier
without metadata conversion or a public API-shape change.

Focused check/catalog/public-analyzer/CLI/plugin tests pass. Shared behavioral
controls check error/result propagation, partial filtering, report restoration,
original analyzer preservation and all-disabled skip; CLI/plugin integrations
retain default, explicit and per-check selection/validation behavior.
Receipt: `.build/goal-adapter-filter-focused.log` (CLI 45.341s; plugin 4.486s).
Canonical validation: `.build/goal-adapter-filter-verify.log`, all eight targets
exit zero. Final architecture validation after documentation:
`.build/goal-adapter-filter-architecture.log`.

Six read-only all-check parent/current scans preserve complete payloads for
lock fixtures (116), private-read fixtures (6) and pinned Skywalking (0).
All exit zero with empty stderr. Pins/scopes/exits/hashes:
`.build/goal-adapter-filter-scans/scans.json`; complete comparison:
`.build/goal-adapter-filter-scans/comparison.json`.
Two traced fixture scans preserve all 3,847 complete records as multisets,
including 232 final decisions; traced diagnostics match untraced payloads.
Receipt: `.build/goal-adapter-filter-traces/comparison.json`.
Reviewed/canonical SHA-256:
`eaa8faf9879a94b17daffe7906e296721b570a8fb4c1e2442a1c7f3111d49bf2`.

The resulting root-wide inventory covers 359 files and 2,286 declarations,
with five whole-body and 33 partial groups. The new tier pair and seven wrapper
windows are removed. The remaining candidates match the earlier distinct-domain
internal dispositions. Counts and normalization are candidate evidence, not
proof that no differently structured semantic duplication exists. Exclusions
retain tests, generated source, fixtures/testdata/vendor and dot/underscore trees.
Graph tools were unavailable; exact source fallback was used.

At the parent source, five successful pinned refresh scopes preserve corrected
Skywalking/Rune silence and reproduce goiardi's two targets among nine findings.
Openase's two targets are unknown opaque ownership; Ferro's target is unknown
command-use-budget-exhausted. These silences are not corrections. Receipt:
`.build/goal-completion-residual-21de/scans.json`. Those scans use the parent
binary and are not represented as final-adapter scan results.
Fresh twelve-row Ferro and three-row goiardi probes run without cutoff:
`.build/goal-completion-ferro-21de-probe/results.json` and
`.build/goal-completion-goiardi-21de-probe/results.json`. Relevant shared proof
source is unchanged by this adapter refactor. They retain implicit-zero/scalar
publication/state-feasibility gaps and the mutable-global/caller-snapshot
boundary. Openase's pinned x/crypto v0.50.0 Session.Close delegates to channel
close, whereas Wait consumes exit status and copy results; Close cannot establish
reader joining. No straightforward one-predicate correction is demonstrated.

No full precision replay, local race run or production FP correction credit.
The broader completion audit remains active for current requirement/evidence
reconciliation; finite child closure and scan counts cannot close the root goal.
