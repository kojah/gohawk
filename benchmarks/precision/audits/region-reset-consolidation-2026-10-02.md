# Region reset consolidation — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.20`. Parent: `333f845`.

## Bounded graph core inventory

Graph tools were unavailable. Source fallback reviewed the complete graph
construction, cache, state, contents, transfer, selected-copy and effect modules,
plus graph alias/identity/observation queries and existing snapshot controls.
The inventory distinguishes the following contracts:

| Family | Authoritative mechanic and disposition |
| --- | --- |
| Construction | One reverse-postorder fixpoint applies transfer; 200,000 transfer allowance and six rounds. Failure discards entry/exit states. Production cache entry rejects nil/bodiless functions before build. |
| Replay | stateAt clones the completed entry and applies the same transfer before the observation instruction. It retains independent replay costs and graph-query locking; no second transfer engine. |
| State joins | Contents include implicit predecessor contents and stale backedge objects. Backing disagreement clobbers; stamp disagreement uses the join identity; escapes/opaque/deferred states accumulate. These are different lattice operations, not duplicate decision policies. |
| Pointee growth | Unknown absorbs a set; stale status accumulates. State, value and may-history widening retain different attribution and destinations. No must guarantee comes from history. |
| Copy/read | Exact entries precede backing/unwritten rules; snapshots preserve source stamps. Copy cycles and more than 64 hops remain unknown. Selected bounded windows are separate from exact single-slot copies. |
| Reset | Allocation repeated full root-subtree clearing. Exact overwrites and opaque clobbering also repeated contents/backing deletion. Consolidated below. |
| Opaque effects | Children are queued before forgetting; clobber stamps, escape reach and defer execution policy remain with effect interpretation. They cannot use full zero-value reset. |
| Query polarity | Alias uses history with bounded placeholder chasing; identity requires one non-stale exact slot; observation queries replay state. These are different questions. Broader structural alias/type and summary-consumer review remains open. |

## Shared reset mechanics

Allocation now calls the existing clearSubtree at its site's root, after the
escaped-loop bailout. regionState.forgetStoredSubtree owns contents/backing
deletion beneath one exact target. clearSubtree additionally deletes clobber
stamps, while opaque clobbering retains stamps after collecting reachable
children. The target prefix cannot erase sibling fields or another region.
Graph values, may-only history, escape maps, summary publication and budgets
remain unchanged. The existing source outliers remain cohesive around their
state/content/effect contracts; this change shrinks transfer and effect bodies.

This is a semantic-preserving consolidation, not a whole-query cost bound or
production FP correction. Graph replay/pointee/type costs and other fact
consumers retain their separate review scope. Ten production FP sites plus Rune
remain unresolved.

The copy review also reproduced a separate nested backing path defect with
compiled SSA: copying *source into copy.nested loses identity between its
unchanged field and source.value. backingOf retains a leading slash below a
nonempty prefix. An ignored normalization prototype passes the parent-failing
assertion. `gohawk-dho.44.11.5.27.21` tracks permanent controls and correction;
this commit does not include the prototype or claim that defect fixed. Receipts:
`.build/goal-backed-path-probe/{parent,corrected}.log`.

## Validation and compatibility

Compiled SSA fresh-loop reset prevents earlier iteration contents from aliasing
a zero field. Existing snapshot/copy/overwrite/opaque/clobber/branch/loop controls
pass, including unavailable backing cycles and deep copy chains. SSA and focused
results are in `.build/goal-region-reset-focused.log`. Three ignored overlays
fail assertions: retaining earlier iteration contents, deleting opaque stamps,
and clearing sibling fields. Receipts:
`.build/goal-region-reset-mutants/{keep-iteration,discard-stamps,forget-siblings}.log`.

Canonical receipt `.build/goal-region-reset-verify.log` covers generation, module
verification, vet, formatting, deadcode, lint, self-dogfood and ordinary tests.
No full precision-regression audit or local race run is performed. The normalized
complete-body scan covers 318 files/2,149 functions with five distinct-contract
groups. It cannot prove partial duplication absent; this source review found
partial deletion loops that it did not flag.

Current `.build/goal-region-reset-reviewed`, SHA-256
`465d570ddf796896b3a796d2dc95f7e5dc43ba258a5e9b6646447924b7cda5cf`.
Parent `.build/goal-exclusive-callers-reviewed`, SHA-256
`f7c1740d5d9d7992c39f194674840f887a7003a79bfd35d11b735a5f3c923627`.
The [ledger](region-reset-consolidation-2026-10-02.tsv) records twelve successful
scans with empty stderr and per-scan exit/hash metadata in
`.build/goal-region-reset-final/`. All 552 lock/resource/goroutine fixture
diagnostics, four pinned XD/goiardi controls and both duplicated SkyWalking FP
sites retain byte-identical JSON. Production modules use readonly mode,
CGO/workspaces disabled, 180-second timeouts and at most two simultaneous scans.
