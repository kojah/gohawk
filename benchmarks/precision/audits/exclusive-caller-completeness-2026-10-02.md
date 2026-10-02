# Exclusive caller completeness — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.19`. Parent: `9eb8851`.

## Reproduced missing caller precondition

Compiled SSA reproduces an invalid exclusivity answer: a private helper has a
fresh synchronous caller, but Go, Defer or a stored callback can hand it shared
objects. The former exclusive list recorded only synchronous static calls and
ignored the other uses. The parent tests fail for all three shapes, while the
fresh-direct and shared-direct controls behave as expected. Receipt:
`.build/goal-exclusive-callers-parent.log`.

Exclusive ownership now consumes the same complete private caller census as
conditional cleanup. Escaped functions, methods, exported functions, empty or
interrupted caller sets cannot prove parameter exclusivity. Every recorded
synchronous argument, including initialization callers, must be fresh and local
at that exact call. The existing 32-caller bound and 20,000-instruction allowance
apply to both consumers. Methods remain unknown because the operand census does
not establish their complete interface/method-value callers. No new method-set
guess, callback contract or asynchronous ownership inference is introduced.

The parallel exclusive map and its separate completion path are removed.
Discovery uses the shared instruction iterator and discards the entire result
at cutoff. The authoritative ownership decision remains parameterExclusive;
its cached answer is scoped to the completed immutable inventory. Local
pre-publication exclusivity and cleanup policy remain separate consumers of
their own evidence. No facts, allowance increase, graph publication or guard
inference changes.

Graph tools were unavailable. Source fallback read caller discovery, exclusive
queries, analyzer wiring, existing fresh/published/shared fixture boundaries and
the shared package/instruction enumeration. This corrects a missed diagnostic,
not any of the ten production FP sites or Rune's guarded-publication question.
The broader graph/alias/type/fact-consumer reviews remain open.

## Controls and scoped comparison

Actual SSA controls cover fresh/shared direct callers, Go/Defer, callback
storage, initialization, 32/33 callers, interface methods and zero allowance.
Existing intermediate-cutoff/fresh-child controls now apply to both consumers.
Focused receipts and dumps: `.build/goal-exclusive-callers-focused-final.log`.
The full lockorder package passes with the minimized callback-cycle fixture;
existing direct-only fresh initialization remains accepted.

Ignoring escaped status and retaining an interrupted caller prefix each fail
assertions, rather than compilation. Receipts:
`.build/goal-exclusive-callers-mutants/{ignore-escape,caller-prefix}.log`.
Canonical validation and final documentation architecture checks are recorded in
`.build/goal-exclusive-callers-verify.log` and
`.build/goal-exclusive-callers-docs.log`. No full precision-regression audit or
local race test runs.

Current `.build/goal-exclusive-callers-reviewed`, SHA-256
`f7c1740d5d9d7992c39f194674840f887a7003a79bfd35d11b735a5f3c923627`.
Parent `.build/goal-lock-callers-reviewed`, SHA-256
`9d6e905d1a46eb9e2ab214aecec33faf330ada9eac45d966fc5169d381572742`.
The [ledger](exclusive-caller-completeness-2026-10-02.tsv) records twelve
successful scans, all with empty stderr and per-scan hash/exit metadata in
`.build/goal-exclusive-callers-final/`.

Lock fixtures add exactly one callback-cycle finding at
`exclusive_callbacks.go:34:2` (107 to 108), with none lost; the full diagnostic
difference is `.build/goal-exclusive-callers-final/lock-fixture-added.json`.
All 444 resource/goroutine fixture diagnostics retain byte-identical JSON.
Four pinned XD/goiardi production controls and both SkyWalking FP sites (each
emitted twice) retain byte-identical output. Production modules use readonly
mode, CGO/workspaces disabled, 180-second timeouts and at most two simultaneous
scans. Scope does not certify all external uses or production precision.

The normalized complete-body scan covers 318 files and 2,148 functions with the
same five distinct-contract groups. It cannot rule out partial duplication.
