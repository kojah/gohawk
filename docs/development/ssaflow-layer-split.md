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
`internal/ssaflow` now has fifty-three direct production files; the remaining
layers and the twenty-file architecture gate are still outstanding.

`make verify` passed after the extraction: generation, module verification,
formatting, vet, dead-code checking, lint, all-checks repository dogfooding and
ordinary tests. The budget recorder tests cover the changed package-qualified
stack-frame filters. These receipts establish this extraction only, not
completion of the repository-wide limit or a fresh external precision audit.

The source graph tools are unavailable in this session. Dependency evidence
comes from type-resolved references and exact source reads; no graph-index
coverage claim is made. Heavy Go work runs serially with caches and temporary
outputs in the task's RAM workspace.
