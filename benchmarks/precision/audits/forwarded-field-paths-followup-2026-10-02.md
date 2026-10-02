# Forwarded field paths — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.12`. Parent: `23164f7`.

## Assessment and change

Actual compiled SSA confirms a saved field load before whole-aggregate
replacement, then a call to a cleanup helper. The parent cannot recognize
completion through that helper. A second control shows the parent incorrectly
credits a nested cleanup of `first` for a target stored in `second`:
`.build/goal-forwarded-fields-parent.log` contains both failed assertions.

The strict-projection proof now reuses the shared read-time parameter path
query for parameter roots. `ProjectionPathProof` retains that non-empty path;
the mapped local carries it into nested path translation instead of trying to
rediscover it from raw SSA. Other roots retain the existing storage-derived
projection policy, whose positive proof may have no exact static path. No new
SSA traversal or ownership engine is introduced.

A nested completion must name the exact path when the caller's target is
contained in the aggregate, as it already must for an owner mapping. Closing
a sibling, or supplying no known nested path, cannot settle that target.
`requiresNestedPath` keeps this mapping requirement beside path translation.
Direct aggregate-method cleanup and loop uncertainty retain their existing
rules. The first canonical gate passes every ordinary test but finds the
combined mapping condition exceeds `instructionCompletes`' complexity limit;
the extraction corrects that gate failure without changing the predicate.

The strict projection retains its `QueryBudget` child cap. Caller/child
exhaustion yields unknown with no path, invalidates enclosing completion memo
answers, and cannot fall through to broader mapping. Existing independent-child
and memo-invalidation controls remain. Cold cutoff/recovery controls cover the
new parameter path and forwarded completion. Production allowances are
unchanged; graph/alias/type internals retain separate costs.

## Controls and scoped review

Heap controls cover saved/wrapped reads, agreeing and conflicting branch
writes, and replacement. Completion controls distinguish the honored field
from its sibling, preserve earlier reads, reject replacement/ambiguous
contents, and publish an exact empty path for the caller's field target.
Deferred contract controls discharge only the original or replacement field
actually selected and export nothing for conflicting writes. Receipts:
`.build/goal-forwarded-fields-controls.log`,
`.build/goal-forwarded-fields-current.ssa.log`, and
`.build/goal-forwarded-fields-resource-controls.log`.

Three overlays fail on assertions:

- Removing the contained-target nested-path requirement incorrectly settles
  both pointer and saved-spill sibling targets:
  `.build/goal-forwarded-fields-sibling-final-counterfactual.log`.
- Dropping the projection path from the local loses valid saved completion and
  the exact exported deferred discharges:
  `.build/goal-forwarded-fields-path-counterfactual.log`.
- Detaching parameter-path visits from the strict child allowance publishes
  positive paths with a one-step request:
  `.build/goal-forwarded-fields-budget-counterfactual.log`.

Graph tools were unavailable. Bounded source review covered strict projection,
parameter access paths, completion mapping/translation, nested completion and
deferred discharge publication. The normalized whole-function scanner sees
315 production files, 2,142 functions and six candidate groups, with no changed
function in a group: `.build/goal-forwarded-fields-duplicate-functions.json`.
It does not establish absence of partial duplication or finish the architecture
review. Deferred storage censuses, other fact consumers and package inventories
remain open.

## Production controls and validation

Final immutable binary `.build/goal-forwarded-fields-reviewed-final`, SHA-256
`cdcb4e5dc1bb7a642aa3ea4d8c7921f130e8d06ef1adaf60fcb061afda2d8f34`.
Pinned XD `b905a14ecfeceaa21a5dde82b52f164d075002ab` scope
`./lib/configparser`; pinned goiardi
`937cae400a92d8036b88ae2f65d93506271c292e` scopes `./datastore ./indexer`.
All-check vet JSON uses readonly modules, CGO disabled, workspace disabled and
180-second limits. Receipts:
`.build/goal-forwarded-fields-{xd,goiardi}-final.json` and `.err`.
Both exit zero with empty stderr and byte-identical output to parent receipts
(838 and 1,476 bytes). The four reviewed controls remain: missing releases at
configparser.go:115/121 and read-lock writes at datastore.go:542/file_index.go:708.

`make verify VERIFY_TIMINGS=1` passes generation, module verification,
formatting, vet, deadcode, lint, self-dogfood and ordinary tests, including
architecture checks. Receipt: `.build/goal-forwarded-fields-verify-final.log`.
No full precision replay or local race run ran. Eleven recorded production FP
sites and Rune remain unresolved; no production FP correction or completed
architecture audit is credited. The broader consolidation goal remains active.
