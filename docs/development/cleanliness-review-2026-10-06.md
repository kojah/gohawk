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

The lifecycle fact file separates schema/accessors (230 lines), descriptions
(259) and imported lookup (69). The prerequisite entry file separates
scheduling/publication (202) from per-function inference (250). All 37 function
signatures and bodies match their originals; fact types, method vocabularies,
text labels, package-marker rules and cleanup-path rationale remain intact.
These are semantic boundaries, not a new package per phase or type.
All eight final canonical gates pass for this layout, and RAM coverage remains
92.6%. AST and validation receipts are retained under
`.build/fact-layout-audit-20261006/`; full profiles and original source snapshots
stay in RAM. No new behavioral fixtures or cumulative precision replay are
needed for unchanged bodies; existing lifecycle and analyzer suites pass.

## Synchronization and heap projection review

The synchronization summary file mixed the evidence contract, package-cache
lifecycle and instruction admissibility. `summary.go` now owns the schema and
completeness contract; `engine.go` owns serialized queries, cache policies and
collection retries; `effects.go` owns ordered instruction accounting and its
passive whitelist. All 25 function signatures and bodies match the original
ASTs. Schema and engine declarations are copied verbatim, with no changed
fact format, query policy or instruction acceptance.

The 605-line `heapmodel/store_heap_summary.go` remains together after a complete
source review. It owns one projection from return-state graphs to externally
named heap evidence. Root/result naming, must/may aggregation, deterministic
fresh-object numbering and truncation share that publication boundary. Call
substitution already lives elsewhere. Its bounded object-hold recursion is
limited by `SummaryPaths`; separating these projections merely to reduce the
line count would scatter one vocabulary and its precision rules.

All eight canonical local verification gates pass after this separation.
Canonical RAM coverage passes at 92.6%, matching the README. AST and gate
receipts are retained in `.build/concurrency-layout-audit-20261006/`; raw
profiles and the original source snapshot remain in RAM. No new behavior
fixtures or precision replay are required for unchanged bodies. No local race
tests were run; hosted checks remain separate.

## Lifecycle capture and tracing review

The lifecycle evidence file mixed proof orchestration with the distinct capture
cell model. `captured_evidence.go` now owns literal retention, unreadable
handoffs, capture uses and immutable imported-argument completion. `evidence.go`
retains one local/imported proof decision path and its outcome-based tracing.
Capture uncertainty, exact completion requirements and the pinned rationale
move with their implementation.

The tracing file mixed evidence events with configuration and timing output.
`configuration.go` owns flag parsing, process-wide destinations and temporary
capture; `timing.go` owns independently enabled measurement output. `trace.go`
retains event selection, candidate attribution and JSONL serialization.
All 50 function signatures and bodies match the original ASTs. Declarations
and initializers are copied without changing flags, output schemas, locking
or disabled paths.

All eight canonical local verification gates pass for this layout. Focused
capture/proof and tracer tests pass, including the CLI cancellation-trace
invocation that validates diagnostic JSON and stable rejected/unknown events.
Standalone package coverage passes for lifecyclefacts (67.5%) and trace
(78.0%); these exclude contributions from other packages and are not comparable
to the preceding 92.6% full-repository profile. No new behavior fixtures or
precision replay are needed for unchanged implementations. Small receipts live
in `.build/evidence-layout-audit-20261006/`; source snapshots and profiles stay
in RAM. No local race tests were run; hosted checks remain separate.

## Resource and cancellation review

Resource acquisition contracts and exclusions remain in `contracts.go` (242
lines); `release.go` owns cleanup and ownership instruction classification
(358). All 22 function signatures and bodies match their originals. The
callback rationale and pinned source link now document the callback function,
while the search-budget rationale documents its constant.

Cancellation proof separates final outcomes/candidate state (163 lines),
instruction classification (259) and private-use resolution. All 28 function
signatures and bodies match immediately after the move. A subsequent traversal
cleanup replaces only `exactLocalValueUse` with the shared fold and its leaf.
The shared fold gains an explicit opaque-phi mode because its ordinary fan-out
would widen this exact-use boundary. Defaults, sibling traversal, leaf policy
and budget availability remain explicit. Actual SSA controls pass both before
and after the change, including converted/nested loads and an opaque merge.
This is traversal consistency, not a new diagnostic or claimed precision gain.

All eight final canonical local gates pass, and full RAM coverage passes at
92.6%, matching the README. The round-2 scoped replays preserve both cancellation
false positives as absent (`nextzhou/workpool` and `tsouza/cerberus`) and both
resource true positives as present (`mongodb/amboy`). Each checkout is clean
and matches its cohort pin, and each final scan requires scannability. The first
Cerberus attempt hit the harness's 180-second timeout during the cold build;
it checked no label and is not counted as a pass. After reclaiming older data
entries from this task's RAM cache, the warmed retry passed under the same
limit. No baselines or labels were edited; this is not a cumulative audit.
Small receipts live in `.build/resource-layout-audit-20261006/`; raw profiles,
source snapshots and the cache-eviction manifest stay in RAM. No local race
tests were run; hosted checks remain separate.

## Region graph cohesion review

`heapmodel/store_regions.go` remains intact after a full source review. Its
object/slot vocabulary, absorbing unknown pointee lattice, stable interning and
stamps, widening and bounded fixpoint define one authoritative memory graph.
Per-instruction replay consumes that same graph rather than a second model.
Instruction effects and query/cache boundaries already live elsewhere. The
constructor's nonempty-body precondition is supplied by `regionsOfFunction`;
the private projection rebuild is reached only for an available/building graph.
The fixpoint's unavailable result discards entry/exit evidence. No source change
or extra behavioral fixture is warranted solely by this file's size.

## Validation receipts

All eight final `make verify` gates pass. The corrected canonical coverage
recipe passes at 92.6%, matching the README, with no stale-package warning.
Raw outputs and AST snapshots remain in the RAM workspace's
`cleanliness-audit/`; small receipts are retained under
`.build/cleanliness-audit-20261006/`. No local race tests or full precision
replay were run for these moves. Hosted checks remain separate from local
validation.

## Remaining review

Three production source files remain to be reviewed for cohesion. These
exceed the 400-line review trigger; that is not a finding that they should be
split. The larger heap projection file has been reviewed above.

- goroutineownership/classify.go
- goroutineownership/obligation.go
- heapmodel/store_regions_effects.go

The returned-view availability question is confirmed and fixed. An actual SSA
fixture storing a closable parameter into a returned owner originally produced
`ReturnedView = 0x1` even when the receiver's `Close` summary was unavailable.
Field-based view inference now withholds that claim for unavailable methods.
Visible private method bodies reuse the same receiver-field release inference
rather than becoming opaque merely because their facts are not exported.
Independent type-only rules retain their existing behavior.

Controls cover unavailable methods, known releasing/no-op methods, type-only
views and private releasing/no-op helpers. The existing same-type wrapper
unit harness now supplies its known no-op method summary. The split-owner
analyzer fixture remains diagnostic after resolving its visible private reset
helper. All eight final verification gates pass for this boundary change.
The scoped round-2 replay on pinned `mongodb/amboy` passes: both retained
resource-lifetime true positives remain present, no baseline drift is reported,
and the scan is required to be scannable. This scope has no false-positive
labels and is not a full cumulative precision audit. Canonical RAM coverage
passes at 92.6%, matching the README. Small receipts are retained in
`.build/returned-view-audit-20261006/`; raw SSA/test output stays in RAM. No local
race tests were run; hosted checks remain separate.

The overall cleanliness goal remains active. Unrelated staged deletion and
untracked files from other sessions are excluded from this task.
