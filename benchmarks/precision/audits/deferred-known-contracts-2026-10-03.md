# Deferred known-call and conditional atomic heap review

Beads: `gohawk-dho.23.8`. Parent: `570048c`.

## Reproduced errors and shared correction

The parent treats pointer/value `CompareAndSwap` as a definite replacement:
compiled SSA controls lose the old value even when comparison can fail. Exact
deferred Mutex.Unlock and RWMutex.RUnlock also lose fresh-owner exclusivity,
because `runDefers` bypasses the standard-call dispatcher.

CAS now uses the same destination, exposure and content-union path as an
uncertain write. Store and Swap preserve definite replacement. Exact deferred
registrations consume the ordinary known-call dispatcher in reverse order;
registration dominance and loop checks still precede it. Captured receiver and
argument SSA values are used, rather than variable values at return. Builtin
policy, summary fallback, unknown-call invalidation and asynchronous exposure
remain separate existing boundaries.

Atomic API contracts live in one focused file. Definite and conditional writes
share one store implementation in `store_regions_writes.go`; general SSA
transfer remains in `store_regions_transfer.go`. No parallel atomic storage
analysis or new summary schema was added. The prior transfer file exceeded the
production size review threshold; extracting its existing store concern keeps
selection and invalidation mechanics together instead of growing that outlier.

## Controls and authoritative evidence

Actual compiled SSA tests cover Pointer/Value Store, Swap and CAS directly and
at deferred execution. ContainsAt retains both CAS possibilities; ContentValue
cannot claim exact replacement. Heap publication independently retains may
edges for CAS and must replacement for Store/Swap, both direct and deferred.
Further controls cover defer receiver/argument capture, conditional/repeated
registration, async exposure, project-defined atomic lookalikes, exact deferred
mutexes and a receiver-retaining project-defined unlock lookalike.

Named return loads can occur between RunDefers and Return. Tests observe the
normal return in that block and exclude the separate recovery return, rather
than assuming adjacent instructions. This observation correction followed an
actual SSA dump, not a production-model workaround.

Parent reproductions: `.build/goal-deferred-contract-parent-atomic.log` and
`.build/goal-deferred-contract-parent-mutex.log` fail the old-value and private
owner assertions respectively. Whole heapmodel tests pass. Four isolated Go
source overlays fail behavioral assertions: CAS forced strong, deferred
known-call bypassed, registration uncertainty bypassed, and async atomic calls
treated as synchronous. Overlay records and logs are in
`.build/goal-deferred-contract-mutants/`; production source was not mutated.

## Scoped diagnostic comparison

The parent binary is `.build/goal-readlock-private-final-reviewed`, SHA-256
`b33072d7dae095c919081b92999e2d2299aa603364b21ebc49be377d48fa87b9`.
The current binary is `.build/goal-deferred-contract-reviewed`, SHA-256
`9cbfe607f32479a1d3dfc9a8df9819a2d8c8a92ae7fa0a59d0fe96045df74d20`.
Successful current receipts from the immediately preceding review are reused
as parent receipts, with the same scopes, pin and immutable binary.

All twelve parent/current receipts are terminal exit 0 with empty stderr.
Merged complete JSON diagnostic payloads are identical: lock/order/path 117,
private read-lock storage 6, resource 318, process 40, goroutine 128, Skywalking 2.
Skywalking is pinned at `e83d5925500a7e63dd55c080a9b1542d6cedaefb`, with a
clean checkout, scope `./pkg/tools/buffer`. The companion TSV records counts,
terminal statuses and the pin; `.build/goal-deferred-contract/scans.json` and
`comparison.json` retain detailed receipts. All checks are enabled. This is a
scoped comparison, not a full precision-regression replay. No candidate tests,
generators or applications were run.

## Validation and limits

The initial canonical run passed generation, module verification, formatting,
vet, dead-code, local dogfood and the full ordinary test suite (151 seconds).
Lint found only a fixture line over 160 columns and an unchecked test dump
error. Both were corrected; final lint reports zero issues and final
format-check, heapmodel and architecture tests pass. Final `make verify VERIFY_TIMINGS=1` is terminal exit 0, including all
canonical gates. Receipts are `.build/goal-deferred-contract-verify.log`,
`.build/goal-deferred-contract-verify-final.log`,
`.build/goal-deferred-contract-lint-final.log`,
`.build/goal-deferred-contract-fmt-final.log` and
`.build/goal-deferred-contract-final-focused.log`. No local race run or full
precision corpus replay was performed.

No recorded production FP correction is credited by this shared-model change.
The seven production sites plus Rune and the full consolidation objective
remain unresolved. CAS success/result correlation and Swap's returned old value
remain unmodeled. A possible heap edge is not cleanup or ownership evidence.
Graph MCP tools are unavailable in this session; discovery and verification
used bounded exact source reads and searches.
