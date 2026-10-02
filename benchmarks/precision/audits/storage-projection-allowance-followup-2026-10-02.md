# Storage projection allowance — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.14`. Parent: `59a3fc2`.

## Consolidation

`Storage.Projection` delegates to one structured stability proof. Its strict
path child retains the default `QueryBudget` cap; independent child cutoff
keeps the budget reason even while the storage request remains available.
Selected wrapper peeling, candidate-address path comparison, observation-window
ordering and nil checks share the storage request allowance. A shortened
ordering or path comparison cannot skip a potentially mutating use and publish
stable storage. Root/source-instruction, wrapper, mutation and exposure rules
are preserved. Embedded-field discovery in `StableFieldContent` also shares
its storage budget.

`SameAccessPathWithin` retains the path-only policy rather than substituting
`ProveIdentityWithin`'s stronger direct-identity shortcut. Identical dynamic
index values remain insufficient for a matching static path. The unused
`SameAccessPath` and `StrictProjectionPath` default adapters are removed;
tests and shared helper references use the authoritative bounded APIs.
No new ownership or traversal engine is introduced. Cycle metadata,
graph/alias/type internals and allocation costs remain separate.

## Regression evidence

`storage_projection_budget_test.go` compiles actual Go SSA and covers read-only,
converted, mutated and escaped roots/slots, mutation after observation, cold
cutoffs, nested embedded setup and independent strict child cutoff with fresh
small-query recovery. The observation-window control retains an actual field
replacement store; its incomplete ordering cannot certify stable storage.
`identity_budget_test.go` distinguishes corresponding static paths from direct
identity of a dynamic-index path.

Three temporary counterfactuals fail on assertions and are restored:

- Detaching embedded setup from the request misses its budget cutoff:
  `.build/goal-storage-projection-embedded-counterfactual.log`.
- Dropping the independent strict child's reason loses budget evidence:
  `.build/goal-storage-projection-child-counterfactual.log`.
- Detaching both observation ordering queries bypasses the allowance:
  `.build/goal-storage-projection-window-counterfactual.log`.

Detaching only the first ordering query did not fail, because the second
still exhausted the shared allowance. The final counterfactual detaches the
whole observation-window family. The first canonical gate found a stale
handwritten helper reference after removing the adapter; it is corrected.
Receipt: `.build/goal-storage-projection-verify.log`.

Graph tools were unavailable. Targeted source review covers the changed
storage queries, path comparator, and selected lifecycle/resource consumers.
The normalized whole-function scan sees 315 production files, 2,143 functions
and six candidate groups, none containing a changed function:
`.build/goal-storage-projection-duplicate-functions.json`. This is not an
exhaustive partial-duplication audit or whole-query cost bound. Broader fact
consumers and deferred graph cell relations remain outside this closure.

## Production controls and validation

Immutable binary `.build/goal-storage-projection-reviewed-final`, SHA-256
`8996a5115db9a60357589059881b5cfbd07029d1374c4f4ba1b3717314b85efc`.
Pinned XD `b905a14ecfeceaa21a5dde82b52f164d075002ab`, scope
`./lib/configparser`; pinned goiardi
`937cae400a92d8036b88ae2f65d93506271c292e`, scopes `./datastore ./indexer`.
Readonly-module, CGO-disabled, workspace-disabled all-check vet scans use
180-second limits. Both exit zero with empty stderr and byte-identical JSON
against the prior immutable binary (838 and 1,476 bytes).
Receipts: `.build/goal-storage-projection-{xd,goiardi}-final.json` and `.err`.
Missing releases at configparser.go:115/121 and read-lock writes at
datastore.go:542/file_index.go:708 remain.

`make verify VERIFY_TIMINGS=1` passes generation, module verification,
formatting, vet, deadcode, lint, self-dogfood and ordinary tests, including
architecture checks. Receipt: `.build/goal-storage-projection-verify-final.log`.
No full precision replay or local race test was run. Eleven production FP
sites and Rune remain unresolved. No production FP correction or complete
architecture certification is credited; broader consolidation remains active.
