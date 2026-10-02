# Nested backing paths — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.21`. Parent: `d9951bc`.

## One canonical relative path

Compiled SSA reproduces lost exact identity after copying an aggregate into a
nested field. backingOf found the correct backing prefix but returned a leading
slash with the relative suffix. The content resolver interpreted that suffix
as a different source slot. Root copies worked because the root fallback
returned a path without the slash; copy propagation separately repaired it.

backingOf now returns a canonical relative path for every prefix. One traversal
handles nested and root lookup; copy propagation consumes the result without
another trimSlash. Nearest-prefix selection, source stamps, snapshot cycles,
64-hop cutoff, and clobber precedence remain unchanged. Neither sibling fields
nor a later source value become the old copied value.

Graph tools were unavailable. Source fallback read backing lookup, both callers
(unwritten and copySubtree), snapshot construction, snapshotBeneath, source stamp
and clobber handling, plus existing backing-cycle/depth and snapshot controls.
No other backingOf caller exists in the current heapmodel source search.
This correction changes local copied-value evidence; it adds no fact vocabulary,
ownership rule, budget increase or callback/participant contract.

## Regression evidence

Permanent compiled-SSA controls cover root/nested/deeper copies, copying a copy,
sibling fields, mutation of the copy or source, a snapshot before source mutation,
and opaque calls affecting either side. The parent fails four unchanged-copy
cases; current passes all ten and the full heapmodel suite. Receipts:
`.build/goal-backing-paths-parent.log` and `.build/goal-backing-paths-focused.log`,
including actual SSA dumps. Existing backing cycles and excessive-depth chains
remain unknown. An ignored overlay retaining the raw suffix fails assertions:
`.build/goal-backing-paths-mutants/raw-relative-path.log`.

Canonical validation receipt `.build/goal-backing-paths-verify.log` covers all
eight local targets, including ordinary tests and self-dogfood. No full
precision-regression replay or local race run is performed. The refreshed
normalized complete-body scan covers 318 production files and 2,149 functions
with the same five distinct-contract groups. It cannot prove partial duplication
absent; this fix removes the separate root lookup and consumer path repair.

## Scoped compatibility and limits

Current binary `.build/goal-backing-paths-reviewed`, SHA-256
`84d25990402cbe18a9a2cfddadb27240843fa48325873974dd9cb2707b966e3b`.
Parent `.build/goal-region-reset-reviewed`, SHA-256
`465d570ddf796896b3a796d2dc95f7e5dc43ba258a5e9b6646447924b7cda5cf`.

The [comparison ledger](nested-backing-paths-2026-10-02.tsv) records six
parent/current scopes and per-scan hash/exit metadata in
`.build/goal-backing-paths-final/`. All twelve scans exit zero with empty stderr.
All 552 fixture diagnostics, four pinned XD/goiardi controls and both duplicated
SkyWalking FP sites retain byte-identical JSON. Canonical verification passes
all eight targets; final documentation architecture checks also pass in
`.build/goal-backing-paths-architecture-final.log`. Production modules use readonly mode,
CGO/workspaces disabled, a 180-second timeout and at most two simultaneous scans.
No production FP correction is credited. Ten production FP sites plus Rune,
structural alias/type and other fact-consumer review remain open.
