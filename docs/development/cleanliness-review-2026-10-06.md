# Codebase cleanliness review — 2026-10-06

Status: in progress. Passing gates are evidence for their enforced rules, not
proof that every implementation file has been reviewed.

## Review criteria

- Shared mechanics have one owning layer; analyzer policy stays local.
- Decision domains use enums; text remains at observation/output boundaries.
- Proof orchestration has one decision path and preserves unknown outcomes.
- Files group one vocabulary and reason to change. Size thresholds trigger
  review rather than arbitrary splitting.
- Public contracts, implementation comments and generated references agree.
- The local verification suite and relevant coverage pass before completion.

Graph tools are unavailable in this session. Discovery and verification use
source inventory, exact reads, AST comparison and the repository's checks;
there is no graph-generation or graph-coverage claim.

## Completed cleanup

The 680-line lifecycle field-summary file mixed field contracts, returned-view
inference and caller retention. `fields.go` now owns field contracts and
result-method composition (388 lines); `returned_views.go` owns borrowed-view
inference (228); `call_retention.go` owns caller-facing retention and summary
availability (91). The misplaced stored-field comment is attached to its
actual helper. All 32 function signatures and bodies match the original ASTs.
Precision rationale comments and pinned links are preserved.

The 513-line CLI output file mixed execution, JSON normalization and rendering.
`delegated_run.go` owns execution (86 lines), `diagnostic_json.go` owns the
schema, normalization and status calculation (172), and
`diagnostic_render.go` owns presentation (284). All 16 function signatures and
bodies match the original ASTs. The existing command-boundary lint exemption
moves to the executor's exact new path. Diagnostic types and the lazy help
initializer retain their original definitions.

Focused lifecycle and CLI tests pass. The canonical local gates pass after
correcting the moved renderer's imports and relocating the existing executor
exemption. Generated lifecycle API references and the architecture note name
the new implementation boundaries. The coverage recipe drops a stale pattern
for the removed `internal/flagvalue` package.

The raw discriminator search finds only expected textual observer interfaces
outside tests; architecture enum checks cover the broader authored-source
scope. The architecture source inventory walks new files and excludes
fixtures, generated source and external trees, with dedicated scope tests.

## Validation receipts

All eight final `make verify` gates pass. The corrected canonical coverage
recipe passes at 92.6%, matching the README, with no stale-package warning.
Raw outputs and AST snapshots remain in the RAM workspace's
`cleanliness-audit/`; small receipts are retained under
`.build/cleanliness-audit-20261006/`. No local race tests or full precision
replay were run for these moves. Hosted checks remain separate from local
validation.

## Remaining review

Twelve production source files still exceed the 400-line review trigger. This
is not a finding that they should be split: each needs a cohesion judgment.

- cancellationownership/proof.go
- concurrencyfacts/summary.go
- heapmodel/store_heap_summary.go
- resourcelifetime/contracts.go
- lifecyclefacts/evidence.go
- goroutineownership/classify.go
- goroutineownership/obligation.go
- lifecyclefacts/fact.go
- heapmodel/store_regions.go
- lifecyclefacts/analyzer.go
- trace/trace.go
- heapmodel/store_regions_effects.go

Further proof-boundary review must distinguish unavailable summaries from
negative evidence, especially where returned-view inference consumes receiver
release masks. No semantic change or new diagnostic is inferred from this
source-review question; it needs real SSA/fact evidence before any fix.

The overall cleanliness goal remains active. Unrelated staged deletion and
untracked files from other sessions are excluded from this task.
