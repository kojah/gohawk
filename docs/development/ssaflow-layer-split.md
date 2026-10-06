# SSA responsibility layers and production-file limit

Status: implementation in progress.

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
| internal/ssaflow | 59 |
| internal/heapmodel | 43 |
| internal/analyzers/resources/resourcelifetime | 37 |
| internal/lifecycle | 28 |
| internal/passes/lifecyclefacts | 28 |
| internal/analyzers/concurrency/lockorder | 27 |
| internal/analyzers/concurrency/goroutineownership | 26 |
| internal/passes/concurrencyfacts | 24 |

The prior type-resolved dependency census groups SSA files into proof/budget,
value, control-flow and call families. The six-file proof/budget family has no
references back to the other families. The other three families form a cycle
under the existing file grouping. Basic CFG queries, instruction metadata and
argument assumptions must be assigned to their owning layer before moving
those families into separate packages.

## Implementation sequence

1. Extract shared proof outcomes, reasons, provenance, observers and budgets
   into `internal/proof`. Migrate consumers directly; retain no re-export facade.
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
live in `internal/proof`, with direct consumer imports and no compatibility
facade. Its seven production files depend on neither SSA nor higher engines.
That extraction left `internal/ssaflow` with fifty-three direct production files; the remaining
layers and the twenty-file architecture gate are still outstanding.

`make verify` passed after the extraction: generation, module verification,
formatting, vet, dead-code checking, lint, all-checks repository dogfooding and
ordinary tests. The budget recorder tests cover the changed package-qualified
stack-frame filters. These receipts establish this extraction only, not
completion of the repository-wide limit or a fresh external precision audit.

## Structural CFG extraction

Raw reachability and instruction ordering, generic keyed work lists,
instruction censuses after a point, and selection from already-feasible edges
now live in `internal/ssaflow/cfg`. These mechanisms have no value, call or path
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
| internal/proof | 7 |
| internal/ssaflow/cfg | 5 |
| internal/ssaflow | 20 |
| internal/ssaflow/calls | 17 |
| internal/ssaflow/path | 16 |

Focused engine, consumer, architecture and documentation tests pass. The four
consumer test fixtures repaired after import-name collisions retain their
original raw SSA source. `make verify` passed: generation, formatting, vet,
dead-code checking, lint, repository self-analysis and the full ordinary suite.
The final direct-file inventory confirms the table above. The other
seven initially over-limit directories and the repository-wide file-limit gate
remain outstanding.

The source graph tools are unavailable in this session. Dependency evidence
comes from type-resolved references and exact source reads; no graph-index
coverage claim is made. Heavy Go work runs serially with caches and temporary
outputs in the task's RAM workspace.
