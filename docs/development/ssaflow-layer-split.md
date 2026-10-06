# SSA responsibility layers and production-file limit

Status: complete.

The requested end state has two requirements: split the SSA machinery into
packages with explicit responsibility layers, and enforce at most twenty
production Go files directly in every repository directory. Tests and fixture
or external trees do not count. Generated production files do count. The limit
has no baseline exemption.

Nested packages are allowed. The limit counts direct children of each
directory, not the sum of files in its subtree. Package boundaries still need
to own a cohesive responsibility and preserve dependency direction.

## Initial inventory

| Directory | Production Go files |
|---|---:|
| internal/engine/ssaflow | 59 |
| internal/engine/heapmodel | 43 |
| internal/analysis/analyzers/resources/resourcelifetime | 37 |
| internal/engine/lifecycle | 28 |
| internal/analysis/passes/lifecyclefacts | 28 |
| internal/analysis/analyzers/concurrency/lockorder | 27 |
| internal/analysis/analyzers/concurrency/goroutineownership | 26 |
| internal/analysis/passes/concurrencyfacts | 24 |

The prior type-resolved dependency census groups SSA files into proof/budget,
value, control-flow and call families. The six-file proof/budget family has no
references back to the other families. The other three families form a cycle
under the existing file grouping. Basic CFG queries, instruction metadata and
argument assumptions must be assigned to their owning layer before moving
those families into separate packages.

## Implementation sequence

1. Extract shared proof outcomes, reasons, provenance, observers and budgets
   into `internal/engine/proof`. Migrate consumers directly; retain no re-export facade.
2. Separate basic SSA and CFG mechanics from value queries, call analysis and
   richer path/obligation proofs. Resolve reverse dependencies at the owning
   mechanism rather than introducing forwarding packages.
3. Bring the other over-limit directories within twenty files by grouping
   implementations around cohesive evidence models. File length remains a
   review trigger; moving arbitrary helpers to new packages is not a remedy.
4. Add the repository-wide architecture gate and fixture checks for its exact
   counting scope, including the twenty/twenty-one boundary and nested folders.
5. Regenerate helper references, run canonical local validation and relevant
   precision replays, and audit every directory before completing the goal.

## First completed extraction

Shared evidence outcomes, reasons, provenance, observers and work budgets now
live in `internal/engine/proof`, with direct consumer imports and no compatibility
facade. Its seven production files depend on neither SSA nor higher engines.
That extraction left `internal/engine/ssaflow` with fifty-three direct production files; the remaining
layers and the twenty-file architecture gate are still outstanding.

`make verify` passed after the extraction: generation, module verification,
formatting, vet, dead-code checking, lint, all-checks repository dogfooding and
ordinary tests. The budget recorder tests cover the changed package-qualified
stack-frame filters. These receipts establish this extraction only, not
completion of the repository-wide limit or a fresh external precision audit.

## Structural CFG extraction

Raw reachability and instruction ordering, generic keyed work lists,
instruction censuses after a point, and selection from already-feasible edges
now live in `internal/engine/ssaflow/cfg`. These mechanisms have no value, call or path
proof dependency. Path guards and obligation state remain with their proofs.
Consumers import the new owner directly; no forwarding API remains in
`ssaflow`. Architecture tests enforce the lower layer's dependency boundary.

The existing queue order, revisit charging, source-slice preservation and
strict dominance tests accompany the extracted implementation. Focused SSA,
CFG, architecture and generator tests pass. `make verify` also passed, covering
generation, formatting, vet, lint, dead-code checking, repository self-analysis
and the full ordinary test suite.
That extraction left `ssaflow` with fifty-two direct production files; `cfg` has five.

## Call and path-proof layers

Value provenance, structural identity, source metadata and natural loops remain
in `ssaflow`. Callee resolution, positional bindings, library contracts and
memoized effects now belong to `ssaflow/calls`. Feasible successors, path guards,
exact counted regions and obligation coverage now belong to `ssaflow/path`.
Consumers import the owners directly; no compatibility aliases or forwarding
functions remain in the value package.

The import direction is enforced: path proofs may consume calls and values;
calls may consume values; values may consume structural CFG mechanics. CFG
mechanics depend only on shared work budgets. Shared proof vocabulary and
source symbol identity sit below these engines. Summary infrastructure and
call-graph guard tests recognize the memo's new package identity and retain
their exact implementation boundaries.

Boolean NOT peeling and integer-literal equality are value mechanics shared by
the higher proofs. Their implementations and cutoff behavior were retained.
Source function and instruction metadata now share `source_metadata.go`.
Private tests moved with their owners; no lower-layer test imports a higher
engine through the package under test.

| Directory | Direct production Go files |
|---|---:|
| internal/engine/proof | 7 |
| internal/engine/ssaflow/cfg | 5 |
| internal/engine/ssaflow | 20 |
| internal/engine/ssaflow/calls | 17 |
| internal/engine/ssaflow/path | 16 |

Focused engine, consumer, architecture and documentation tests pass. The four
consumer test fixtures repaired after import-name collisions retain their
original raw SSA source. `make verify` passed: generation, formatting, vet,
dead-code checking, lint, repository self-analysis and the full ordinary suite.
The final direct-file inventory confirms the table above. The other
seven initially over-limit directories and the repository-wide file-limit gate
were addressed by the next step below.

## Repository-wide consolidation and gate

All seven remaining directories now contain twenty direct production Go files.
Related query fragments were combined inside their existing packages; no new
adapter packages, forwarding functions or evidence rules were introduced.
Declaration-token comparison against the preceding commit verifies identical
implementation bodies and retained original comment groups in all seven scopes.
Two pre-import responsibility comments were restored next to their owning
declarations during that review.

The larger consolidated files received the required cohesion review:

| File | Responsibility retained together |
|---|---|
| heapmodel/store_regions_effects.go | Opaque storage effects and the atomic, mutex and builtin contracts that bound them |
| heapmodel/store_regions_content.go | Versioned slot content, writes and bounded aggregate slot layout |
| heapmodel/store_heap_summary.go | Return projection of completed graph states; bounded result-copy helpers remain separate |
| heapmodel/store_projection.go | Exact aggregate paths and their observation-time stability |
| heapmodel/query.go | Public and internal graph relationship queries with explicit polarity and availability |
| heapmodel/store_cells.go | Occupant resolution and point-in-time or lifetime cell guarantees |
| lockorder/callee_locks.go | Callee lock inventories and exact binding onto caller receiver paths |
| resourcelifetime/flow.go | Initialization and expansion of the same resource activation/release state |
| resourcelifetime/ownership.go | Possible holder discovery and exact storage destination disposition |

These files exceed the 400-line review trigger because a complete evidence
family is kept together. Their existing functions were not enlarged. Distinct
lifetimes and boundaries remain separate: graph caching, graph state,
requirements, caller substitution, rendering, acquisition-error correlation,
wrapper-chain proofs and result ownership retain their own files. The separate
500-code-line lint limit also remains enforced: result-copy helpers were kept
separate when their merge would have exceeded it; no lint budget was raised.

`TestProductionFileLimit` walks the maintained repository through the shared
source inventory and fails any directory above twenty direct production files.
The layout view includes generated source and inactive build variants, while
authored conformance views retain their generated-file exclusion. Fixtures
prove the twenty/twenty-one boundary, independent nested allowances, generated
file counting, overlapping-root deduplication, and test/external exclusions.
There is no baseline exemption. The focused gate and full module compilation
pass. Canonical validation and the completion audit are recorded below.

## Completion audit

The final maintained-source inventory contains forty production directories,
with a maximum of twenty direct Go files and no violations. The strict
architecture gate and its counting fixtures pass. SSA responsibility and
dependency boundaries are enforced by the layer tests; no compatibility facade
was retained. Generated helper references and contributor instructions describe
the new owners and counting scope.

`make verify` passed generation, module verification, formatting, vet, dead-code
checking, lint, all-checks repository self-analysis and the full ordinary test
suite. Canonical coverage, now including `internal/engine/proof`, passed at 92.6%.
Local race testing was not run; it remains a CI gate.

Round 2 was replayed against the pinned Workpool, Amboy, Vekil and Cerberus
repositories with all checks enabled and scannability required. Five of five
reviewed false positives remain absent and three of four reviewed true positives
remain present. The replay is not wholly green: Cerberus's resource-lifetime
label at `test/e2e/migration/tiers/tier2-ruler/receiver/main.go:133:15`, last
confirmed at `9613f2d` on 2026-09-06, is missing. Separate binaries from both
`7e613ad2` (before the directory consolidation) and `242d9471` (before the entire
SSA layer split) miss the same pinned label. This is an existing recall gap;
the label was not changed to make the replay pass.

Validation artifacts are retained in the task's RAM workspace at
`/dev/shm/gohawk-perf-01a0f86c/layer-split`: `verify-file-limit-final.log`,
`coverage-summary.out`, `consolidation-equivalence-final.log`,
`final-file-inventory.json`, `replay.log`, and both baseline Cerberus logs.
The independent declaration-token comparison verifies that the final seven
directory consolidations retained their implementation bodies and original
comment groups. Every requirement of the requested layout and file limit is
implemented and verified at commit `07a6c44f`. The user subsequently accepted
the broader internal hierarchy described below.

## Accepted internal hierarchy follow-up

Status: complete; final local verification passed.

Packages now live under five top-level directories: `engine`, `analysis`,
`reporting`, `testsupport` and `cli`. Repository-wide conformance checks live
in `testsupport/architecture`. The first four top-level directories are
organizational containers, with no umbrella package. Engine mechanics remain
below domain passes and analyzer policy; reporting and test support keep their
own roles. Public analyzer registration remains at the existing public path.

Imports, source inventories, synthetic architecture fixtures, generation paths,
coverage scopes, scripts and maintained helper references were migrated to
their new owners. The layer resolver recognizes containers before applying the
same dependency rules, with fixtures for real hierarchical paths. Full module
compilation, focused architecture/generator tests and full `make verify` pass.
The final inventory contains 40 production directories with a maximum of 20
files directly in each. The hierarchy precision replay retains the same five
accepted false-positive labels and three true-positive labels, with only the
existing Cerberus recall gap described above.

Coverage measured 92.6% before the user requested CI-only instrumentation.
Coverage and race testing now run only in GitHub Actions: Make rejects local
instrumented targets before prerequisites start, and repository/agent policy
prohibits direct instrumented Go commands or spoofing CI markers. The guard
is tested with a fake Go command and CI dry runs, without executing local
instrumentation. Hierarchy receipts live under
`/dev/shm/gohawk-perf-01a0f86c/internal-hierarchy`.

The source graph tools are unavailable in this session. Dependency evidence
comes from type-resolved references and exact source reads; no graph-index
coverage claim is made. Heavy Go work runs serially with caches and temporary
outputs in the task's RAM workspace.
