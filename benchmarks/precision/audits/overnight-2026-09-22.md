# Overnight audit: 1,250 repositories

Completed the initial 250-repository batch and four further batches of 250.
All 1,250 selections are distinct repositories pinned to full commits. Every
one of the 2,165 emitted findings has a source-review verdict and explanation.
These are static source judgments, not candidate test runs or runtime incidents.

| Batch | Complete / incomplete | Findings | True positive | False positive | Inconclusive | Verified FP corrections |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| [51](batch-51.md) | 193 / 57 | 421 | 325 | 85 | 11 | 24 |
| [52](batch-52.md) | 200 / 50 | 455 | 341 | 107 | 7 | 2 |
| [53](batch-53.md) | 201 / 49 | 386 | 322 | 48 | 16 | 0 |
| [54](batch-54.md) | 203 / 47 | 311 | 242 | 65 | 4 | 1 |
| [55](batch-55.md) | 194 / 56 | 592 | 520 | 69 | 3 | 10 |
| **Total** | **991 / 259** | **2,165** | **1,750** | **374** | **41** | **37** |

There were 1,548 selected module entries and 584 quiet complete scans.
"Complete" means the recorded selected-module profile completed, not that
every module or execution mode in a repository was analyzed. Incomplete scans
are not clean; quiet scans do not establish absence of bugs or measure recall.

## What changed

Seven focused implementation commits improve existing evidence boundaries:

- `c04189c`: global, merged and retained resource ownership uncertainty.
- `0182cad`: exact parent-context cancellation as an ownership boundary.
- `f50a566`: readiness-only Done signals, aggregate completion and bounded
  worker-context ownership.
- `c9609c6`: exact errors.As acquisition-failure guards.
- `88a6b90`: immediate Process guards after successful Cmd.Start.
- `f8365c0`: effective Go 1.22+ per-iteration range-variable semantics.
- `7abe771`: synchronous bound cleanup callbacks in shared cross-package
  lifecycle summaries.

The 37 corrections count original finding locations, not distinct analyzer
defects. Pinned replays verify each correction; original findings and verdicts
are preserved separately. Batch 54's process correction reuses the batch 52
implementation. Audit-record commits are separate from implementation commits.

No analyzer or check was retired, disabled or demoted. A few diagnostic fixtures
and one historical early-Done label were removed as explicit accepted coverage
gaps, documented in batch 51. These precision improvements do not prove recall
was preserved for every unsupported pattern.

## What remains

337 reviewed false-positive locations remain unresolved. Each batch assessment
groups the missing evidence and records bounded directions or conservative
limits; they are not presented as fixed. The recurring issues are correlated
cleanup and acquisition paths, ownership through wrappers and registries,
exact lock identities and causal worker completion, and finite protocol or
loop cardinality. Expanding these requires reusable evidence, not accumulating
framework-name exceptions.

Findings are clustered: 175 batch-55 operand-order findings come from one
generated gotgbot pattern, and Sponge contributes another 86 findings across
several families. Raw totals are not counts of independent bug classes. The
batch-55 assessment also distinguishes a feasible queued-writer hazard from
nearby lock-order alerts ruled out by exact instances or outer synchronization.

## Validation and scope

Canonical `make verify` passes. Changed-package race tests pass, including the
final lifecycle-summary and resource tests. The final shared-summary change
retains all 29 labels in rounds 55 and 56; round 57 passes twelve new labels.
Earlier corrections have their focused fixtures and pinned control replays.
Ledger checks validate exact finding keys, check IDs, full pins, checkout HEADs,
source lines and byte columns. Original scan binaries were never overwritten.

The optional architecture-package race gate is not claimed clean: a Go 1.27 /
x/tools type-loader race also reproduces on unchanged `0480106`. Batch 52
documents that baseline limitation. No unrelated dependency workaround was made.

Only static candidate analysis ran; no candidate tests, generators or
applications were executed. Fresh Go 1.26+ candidates were insufficient for the
additional thousand, so compatible older declarations were included newest
first. Selection manifests retain those criteria, full pins and exclusions.
No upstream bug reports or external project changes were made.
