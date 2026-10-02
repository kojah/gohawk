# Lock caller inventory consolidation — 2026-10-02

Beads: `gohawk-dho.44.11.5.27.18`. Parent: `fe0597f`.

## One package discovery, distinct preconditions

`runLockOrder` previously walked package bodies once for conditional-release
callers and again for exclusive-ownership callers. `collectLockCallers` now
feeds both views from one instruction census. Discovery lives in `callers.go`;
the conditional return proof and exclusive acquisition proof retain their own
policies. No unused constructor that rescans bodies remains.

The conditional view retains initialization, generated bodies selected by
PackageFunctions, private non-method function operands, debug-reference exclusion,
32 synchronous callers and escaped status for other uses. Its existing
20,000-instruction allowance charges every instruction, including initialization.
Exhaustion discards the entire conditional map. The separate exclusive view
retains all synchronous static calls outside initialization, including exported
callees; it continues to completion after conditional exhaustion. It adds no
Go/Defer, dynamic or interface edges. No partial conditional caller list can
become a cleanup guarantee. Neither view is a guard-to-field proof.

Graph tools were unavailable. Source fallback read both original collectors,
analyzer wiring, parameterExclusive, the read-lock-write proof and shared
PackageFunctions/instruction enumeration. The SkyWalking pinned Buffer source
still writes head/current under the event-list read lock and elsewhere without
that lock. Those unlocked writes do not prove confinement. Both sites remain
unresolved; no suppression is added. Source anchor:
https://github.com/apache/skywalking-rover/blob/e83d5925500a7e63dd55c080a9b1542d6cedaefb/pkg/tools/buffer/buffer.go#L629-L654

The exclusive precondition currently examines its direct synchronous sites
without the conditional view's escaped-function filter. A separate follow-up,
`gohawk-dho.44.11.5.27.19`, must verify whether callback/asynchronous uses can
invalidate an all-fresh-direct-caller answer. This refactor preserves that
policy; it does not certify caller completeness or solve Rune publication.
Graph build/replay, pointee/alias/type internals and other fact consumers remain
open. Ten production FP sites plus Rune remain unresolved.

## Validation

Compiled SSA controls pin initialization scope, direct versus handed-on function
operands, Go/Defer escape, exported functions, 32/33-call boundaries, nil entries,
every tested intermediate allowance cutoff and fresh-child recovery. An exhausted
conditional census leaves the full independent exclusive inventory intact.
SSA and focused walk results: `.build/goal-lock-callers-focused.log`.

Three ignored overlays fail assertions rather than compilation: retaining the
conditional prefix, admitting initialization into exclusive callers, and stopping
exclusive discovery at conditional cutoff. Receipts:
`.build/goal-lock-callers-mutants/{retain-prefix,init-exclusive,truncate-exclusive}.log`.

Canonical `make verify VERIFY_TIMINGS=1` passes all eight targets, including
ordinary tests and architecture checks (`.build/goal-lock-callers-verify.log`).
No full precision-regression audit or local race run was performed. The refreshed
normalized complete-body inventory covers 318 files and 2,149 functions with
the same five distinct-contract groups. It does not rule out partial duplication.

## Scoped receipts

Current `.build/goal-lock-callers-reviewed`, SHA-256
`9d6e905d1a46eb9e2ab214aecec33faf330ada9eac45d966fc5169d381572742`.
Parent `.build/goal-cycle-inventory-reviewed`, SHA-256
`07c3a4903ddb910b139fd828d593b65480f2f9d5724c53d3105e8ff555eb19f8`.

The [ledger](lock-caller-inventory-consolidation-2026-10-02.tsv) records six
parent/current scopes. All twelve scans exit zero with empty stderr and per-scan
hash/exit metadata in `.build/goal-lock-callers-final/`. All 551 lock/resource/
goroutine fixture diagnostics are byte-identical. The four pinned XD/goiardi
lock controls preserve their exact keys and JSON. SkyWalking preserves both
FP sites, each emitted twice, with byte-identical 3,095-byte JSON. Production
modules use readonly mode, CGO/workspaces disabled, a 180-second timeout and at
most two scans concurrently. No FP correction is credited.
