# Completion spill paths follow-up — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.10`. Parent: `4b77c5a`.

## Change and policy

Parameter spill paths now expose `AccessPathFromParameterWithin`, delegating
the direct path and selected spill roots to shared SSA path queries. Its
whole-written-cell test uses `WholeWrittenCellWithin`, which delegates to the
same existing referrer and selection validation. Default adapters retain nil
allowance for consumers without an existing request budget. No independent
path traversal or stronger spill identity is introduced.

Completion receiver matching, receiver path recording and supplied-path
translation share the completion request allowance. Exact cleanup receiver
matching charges each existing transparent wrapper visit, preserving its
rejection of phi, load and further projection forms. A budget cutoff supplies
no matching receiver or published path. The existing completion coverage and
memo composition remain authoritative for request availability.

`heapmodel.AccessPathOf` became unreachable and is removed: direct paths use
`ssaflow.AccessPathStepsWithin`, or the default shared adapter where needed.
The generated reference follows this cleanup.

Graph tools were unavailable. Bounded source review covered the heap access-
path adapter, shared whole-written-cell validation, completion mapping and
completion path translation. Deferred storage referrer/path censuses,
package-level consumers without request budgets, graph/alias/type internals
and path rendering/allocation remain separate work. This is not a whole-query
bound or a completed architecture audit.

## Actual SSA and controls

Heap controls accept direct, spilled, copied and constant-index paths. Field
replacement, cell exposure, dynamic indexes and unrelated roots stay rejected.
Every cold cutoff publishes neither a path nor a known empty path. A whole-
cell child cutoff leaves its parent available and a fresh child recovers.
Receiver controls accept interface/type wrappers while rejecting phis and
loads; a one-step allowance cannot cross the wrapper chain.

Focused heap and lifecycle tests pass in
`.build/goal-completion-spill-controls-final.log`. Three overlays fail on
assertions: detaching whole-cell visits, detaching direct-path visits, and
removing exact wrapper traversal charges. Receipts:
`.build/goal-completion-spill-{whole,path,receiver}.log`.

Actual SSA dumps succeed with empty stderr:
`.build/goal-completion-spill.ssa` confirms a whole store from a by-value
parameter followed by field selection/load;
`.build/goal-completion-spill-wrapper.ssa` confirms ChangeType followed by
MakeInterface. Sources use the same test shapes and execute no programs.

The first canonical gate identifies the now-unused AccessPathOf adapter;
`.build/goal-completion-spill-verify.log` is retained as a failed receipt.
No production limit or padded recovery control was altered in this iteration.

## Scoped production controls

Initial immutable binary `.build/goal-completion-spill-reviewed`, SHA-256
`22cba0c21953f8df90282f745a256f6ee834a91404472826f2b47f721db65862`.
Initial receipts `.build/goal-completion-spill-{xd,goiardi}.json` and `.err`.

Pinned XD `b905a14ecfeceaa21a5dde82b52f164d075002ab` scope
`./lib/configparser` and goiardi `937cae400a92d8036b88ae2f65d93506271c292e`
scopes `./datastore ./indexer` use readonly modules, CGO disabled, workspace
disabled, 180-second limits and all-check vet JSON. Both initial scans exit
zero with empty stderr and byte-identical output to the preceding identity
mapping receipts (838 and 1,476 bytes). The four reviewed controls are missing
releases at configparser.go:115/121 and read-lock writes at
datastore.go:542/file_index.go:708.

Eleven recorded production FP sites and Rune remain unresolved. No production
FP correction is credited; no full precision replay or local race run belongs
to this iteration. Broader consolidation remains active.

## Final validation

Final immutable binary `.build/goal-completion-spill-reviewed-final`, SHA-256
`280f3636dbb36b813cec15086e3ba5e24410cdbd0dbb7d8105d74f6d11404af1`.
Final receipts `.build/goal-completion-spill-{xd,goiardi}-final.json` and `.err`
exit zero with empty stderr and byte-identical JSON to the preceding identity
mapping receipts. All four reviewed controls remain.

`make verify VERIFY_TIMINGS=1` passes every local target, including ordinary
tests and architecture checks, generation, module verification, formatting,
vet, deadcode, lint and self-dogfood. Final receipt:
`.build/goal-completion-spill-verify-final.log`. No full replay ran.

## Next semantic correction

A separate source/SSA probe found a pre-existing whole-cell replacement gap:
`func replaced(b,c box)*int { b=c; return b.first }` is still named as
`field:0` beneath original b. The dump
`.build/goal-completion-spill-replaced-whole.ssa` shows both whole stores to
one cell. A separate test-only overlay fails on that incorrect path in
`.build/goal-completion-spill-replacement.log`; this is not a passing control
or a change to the checked-in fixture. Beads `.27.11` tracks the semantic
correction and consumer facts. The current consolidation preserves the old
spill policy; it does not correct this newly identified gap.
