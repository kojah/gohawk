# Bounded source-slot naming and lifecycle argument consumption

Bead: `gohawk-dho.23.21.4`. Parent: `a2b941c`.

This change consolidates partial duplicate groups 24 and 36 from the bounded
shared-layer review. Graph tools were unavailable; evidence comes from exact
production source and actual SSA controls. This is not a repository-wide
semantic completeness claim.

Lifecycle field-result transfer and returned deferred-cleanup queries now
share argument-first possible-alias dispatch. Operand orientation, query order
and short circuit remain unchanged. Containment does not establish consumption
for these queries. The callers still own their distinct field/defer evidence.

Heap current-state, history and escape projections share root/path naming and
the existing source-slot depth bound. State counts and truncation, history
membership and escape coverage remain separate. Forwarded value targets, reads
and requirements retain their separate naming contracts.

## Validation

Actual SSA controls exercise a later matching argument, unrelated arguments and
containment-only handoffs through both lifecycle APIs. Heap controls exercise
source edges at the depth limit, omitted over-depth source/escape slots,
forwarded over-depth target identity and retained history after overwrite.
Both new test groups pass with parent production files restored by an overlay.
A containment-broadening overlay fails both containment assertions; a widened
path-bound overlay fails the over-depth assertion. Both failures are test
assertions, not build failures.

Focused heapmodel/lifecycle tests and `make fmt` pass. Canonical
`make verify VERIFY_TIMINGS=1` exits 0: generation, module verification, vet,
formatting, deadcode, lint, self-dogfood (57 seconds) and ordinary tests
(94 seconds). The canonical binary matches the frozen reviewed binary.

Six fixture scopes and two pinned production scopes were scanned with every
check enabled. Sixteen terminal receipts have exit 0 and empty stderr; complete
merged parent/current diagnostic payloads are identical:

| Scope | Findings in each version |
| --- | ---: |
| cancellationownership + diagnostic | 40 |
| lockorder + ordercycles + readlockpaths | 117 |
| privateread | 6 |
| resourcelifetime | 318 |
| processownership + processchoices | 41 |
| goroutineownership + closurechoices | 128 |
| Rune `./internal/ide/idepkg` | 1 |
| Skywalking `./pkg/tools/buffer` | 2 |

Rune pin: `3e2165f8983280542c985947378dfa740a397d03`, with CGO enabled.
Skywalking pin: `e83d5925500a7e63dd55c080a9b1542d6cedaefb`.

Parent binary SHA-256:
`9329802541bddbe68a19a11a29754f33a0352dfc7a90e38cb83177096d5760a2`.
Reviewed/current binary SHA-256:
`24f3e531a5ca7ed81b8c6ab8dbd84f28b6d7a8630e6400626b35df0850a50aec`.
Local receipts: `.build/goal-slot-naming-arguments/{scans,comparison}.json`;
canonical log: `.build/goal-slot-naming-arguments-verify.log`.

No full precision regression or race run was performed. No production
false-positive correction is established. Eight previously identified
production locations and broader semantic consolidation remain unresolved.

After updating the development notes and this record, the final
`go test ./internal/testsupport/architecture` exits 0 (2.293 seconds).
