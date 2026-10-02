# Whole-aggregate spill replacement — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.11`. Parent: `241d4b3`.

## Correction

The original parameter-path query named `b=c; return b.first` beneath both
parameters because a whole-written cell could contain either aggregate.
`AccessPathReadWithin` extends the existing static-path traversal with its
nearest-root load. Writes-only storage contents and identity now establish
which parameter occupies the spill at that read. Saved reads survive later
replacement and wrapping; agreeing writes retain identity. Replacement,
conflicting writes and exposure cannot publish the original parameter path.
Address-only paths require every whole write to agree, since there is no read
snapshot. No second path or reaching-write engine is introduced.

Completion also previously fell back to possible derivation when exact path
mapping failed. Receiver proofs now separate exact matches from possible
cleanup. Only exact matches settle a caller's field target; possible cleanup
inside a cycle preserves loop uncertainty. The aggregate's own cleanup method
remains accepted when its original receiver path is proven empty. This retains
the existing resource loop-helper boundary without claiming an exact element
was closed.

Supplied allowances cover path, content and identity questions. Cold cutoff
publishes neither a path nor read metadata; fresh queries recover. Writes-only
queries never build a points-to graph. Graph tools were unavailable; bounded
source review covered the shared path traversal, parameter spill adapter,
completion receiver proof and cycle handling. Graph/alias/type internals,
deferred storage censuses and other fact consumers remain separate work.

## Verification

Controls cover original/replacement parameters, earlier and wrapped reads,
restoring the original aggregate after a replacement read, nested pointer
reads, agreeing and conflicting branches, address-only paths, ordinary caller
completion, exported cleanup contracts, aggregate methods, dynamic loop
uncertainty, cold cutoffs and recovery. Actual compiled helper SSA is retained
in `.build/goal-spill-replacement-completion-current.ssa.log`.
Focused receipt: `.build/goal-spill-replacement-controls-reviewed.log`.
Affected resource fixtures: `.build/goal-spill-replacement-resource-controls.log`.

Three test-only overlays fail on assertions:

- Bypassing the content check names replacement fields beneath the original
  parameter and exports incorrect discharge contracts:
  `.build/goal-spill-replacement-guard-counterfactual.log`.
- Crediting possible derivation as exact completion closes the wrong target:
  `.build/goal-spill-replacement-fallback-final-counterfactual.log`.
- Selecting the outermost load loses the valid saved nested-pointer read:
  `.build/goal-spill-replacement-outer-counterfactual.log`.

The first gate exposed an unchecked test dump error and four resource loop
uncertainty regressions. The second gate captured an intermediate unsupported
probe that constructed a new collection inside the callee; that probe supplied
no derivation witness. The final control ranges over the supplied array,
matching the existing supported boundary. Failed receipts remain in
`.build/goal-spill-replacement-verify{,-final}.log`.

Final `make verify VERIFY_TIMINGS=1` passes generation, module verification,
formatting, vet, deadcode, lint, self-dogfood and ordinary tests, including
architecture checks. Receipt: `.build/goal-spill-replacement-verify-reviewed.log`.
Production allowances are unchanged. No full precision replay or local race
run ran.

## Pinned production controls and remaining work

Final immutable binary `.build/goal-spill-replacement-reviewed-final`, SHA-256
`8abaf91f43cc3c4dfb37b80f60eb6c26130a9edf28325ce5870ccaf0098aa4f7`.
Pinned XD `b905a14ecfeceaa21a5dde82b52f164d075002ab`, scope
`./lib/configparser`; pinned goiardi
`937cae400a92d8036b88ae2f65d93506271c292e`, scopes `./datastore ./indexer`.
Both all-check vet JSON scans exit zero with empty stderr and byte-identical
output to the parent spill-path receipts (838 and 1,476 bytes). Readonly modules,
CGO disabled, workspace disabled and 180-second limits were used. Receipts:
`.build/goal-spill-replacement-{xd,goiardi}-final.json` and `.err`.
Missing releases at configparser.go:115/121 and read-lock writes at
datastore.go:542/file_index.go:708 remain.

Separate parent/current overlays reject forwarded replacement cleanup but also
fail to recognize a valid saved field forwarded to a nested helper. This is a
pre-existing coverage gap tracked in `.27.12`; receipts
`.build/goal-spill-replacement-nested-{probe,parent}.log` are failed probes,
not passing controls. Eleven recorded production FP sites and Rune remain
unresolved. No production FP correction or complete architecture audit is
credited; the broader consolidation goal remains active.
