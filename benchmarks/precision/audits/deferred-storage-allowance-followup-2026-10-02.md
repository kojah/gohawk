# Deferred storage allowance — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.13`. Parent: `cabda09`.

## Consolidation

Deferred binding lookup, direct-store census and target-relative store checks
now share the completion request allowance. The full store census precedes
the intervening-write proof. Candidate/other-store visits and instruction
ordering consume the same allowance; a shortened negative search cannot
certify the absence of a replacement. The existing alias and conditional
acquisition policy is preserved, not strengthened into exact identity.

`StoreMayFollowWithin` retains the existing fresh-allocation rule while sharing
dispatch, instruction-index scans and queued CFG visits with its caller.
`StableContent`, `StableFieldContent` and deferred target mapping use the same
query and reject unavailable ordering before publishing positive evidence.
The unused unbounded adapter is removed; no second CFG or storage engine is
introduced. Cycle metadata, graph/alias/type internals and allocations remain
separate costs; this is not a whole-query time or memory bound.

Stable deferred binding lookup keeps the default `QueryBudget` storage child
cap. Its structured cutoff reason reaches completion's mapping layer, which
invalidates the enclosing memo even when the request remains available. The
generic storage query stays below completion; it owns no completion memo.

## Controls and failed intermediates

Actual compiled SSA controls preserve conditional acquisition, reject later
and intervening writes, distinguish fresh/reused loop cells, exercise an
80-use census, and require cold cutoffs to publish no witness or stable value.
A bound-method field receiver exercises an independent storage-child cutoff
under a larger request: a shortened mapping cannot be cached, and a fresh
small stable lookup recovers. The initial literal-capture probe instead bound
a spill cell; it was corrected to the actual field-address method binding.

Receipts: `.build/goal-deferred-storage-controls-current.log`,
`.build/goal-deferred-storage-layer-controls.log`,
`.build/goal-deferred-storage-current.ssa.log`, and
`.build/goal-deferred-storage-order.ssa.log`.

Four test-only counterfactuals fail on assertions:

- Detaching the full census publishes a target-store witness with a short
  allowance: `.build/goal-deferred-storage-census-layer-counterfactual.log`.
- Detaching direct-store visits misses the cutoff on a zero-initialized cell:
  `.build/goal-deferred-storage-direct-layer-counterfactual.log`.
- Detaching store-order work bypasses the caller's allowance:
  `.build/goal-deferred-storage-order-final-counterfactual.log`.
- Keeping an independently cut mapping caches a shortened memo answer:
  `.build/goal-deferred-storage-memo-layer-counterfactual.log`.

The first gate identifies captured-mapping and test complexity; the storage
availability composition and test assertions are extracted without changing
the policy. The second gate identifies completion memo ownership placed in
the lower storage family. The final composition keeps the storage query below
completion and memo invalidation in the higher mapping family. Failed receipts
remain in `.build/goal-deferred-storage-verify{,-final}.log`.

Graph tools were unavailable. Bounded source review covers deferred mapping,
cell queries, store ordering and both stability consumers. The normalized
whole-function scanner sees 315 production files, 2,144 functions and six
candidate groups, none containing a changed function:
`.build/goal-deferred-storage-duplicate-functions-layer.json`. It is not proof
of absence of partial duplication. Remaining storage projection setup is
tracked in `.27.14`; graph cell-relation and other fact-consumer costs remain.

## Pinned production controls and final gate

Final immutable binary `.build/goal-deferred-storage-reviewed-layer`, SHA-256
`179948487a082ce618ab461569ea6e62067e588d8e0ae0734ce8195795188b4d`.
Pinned XD `b905a14ecfeceaa21a5dde82b52f164d075002ab` scope
`./lib/configparser`; pinned goiardi
`937cae400a92d8036b88ae2f65d93506271c292e` scopes `./datastore ./indexer`.
All-check vet JSON uses readonly modules, CGO disabled, workspace disabled
and 180-second limits. Receipts:
`.build/goal-deferred-storage-{xd,goiardi}-layer.json` and `.err`.
Both scans exit zero with empty stderr and byte-identical output to parent
receipts (838 and 1,476 bytes). Missing releases at configparser.go:115/121 and
read-lock writes at datastore.go:542/file_index.go:708 remain.

`make verify VERIFY_TIMINGS=1` passes generation, module verification,
formatting, vet, deadcode, lint, self-dogfood and ordinary tests, including
architecture checks. Receipt: `.build/goal-deferred-storage-verify-layer.log`.
Production limits are unchanged. No full precision replay or local race run ran.
Eleven recorded production FP sites and Rune remain unresolved; no production
FP correction or complete architecture audit is credited. Broader consolidation
remains active.
