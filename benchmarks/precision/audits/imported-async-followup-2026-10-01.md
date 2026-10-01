# Imported asynchronous resource exposure

Correction `6c68806` removes golang/debug's reviewed profiling-writer false
positive from the [22-site scoped replay](pending-production-fp-replay-2026-10-01.md).
Four of those sites now have verified corrections, leaving 18 unresolved.
Original audit verdicts and replay snapshots remain unchanged.

At [the pinned writer](https://github.com/golang/debug/blob/ac862fd6552b739f50ba812382eed75745a129b1/cmd/viewcore/main.go#L820-L829),
`StartCPUProfile` receives the acquired file. Its imported heap summary already
records `P0 escaped async some`, but resource lifetime's async classifier read
only local call effects. `ClaimAsynchronouslyExposes` now derives that positive
exact-parameter effect from the existing heap projection. One classifier
predicate consumes both imported claims and local effects. This adds no fact
field, wire format version, pprof-name contract, or new summary inference.

The handoff makes ownership unknown, never guaranteed cleanup or transfer.
It is a may-claim, so callee paths without a launch can hide genuine leaks,
as with the existing local async boundary. Synchronous borrowing, another
parameter's async exposure, child-field effects, missing heaps and truncation
alone do not qualify. A caller return bypassing the handoff remains diagnostic.
The imported async fixture reproduces before the change and passes afterward;
heap-claim and analyzer controls cover those distinctions. The trace assertion
uses the existing asynchronous-exposure reason and unknown label.

## Scoped evidence

The parent binary implements `65f3a0a`, SHA-256
`e0d9a9b99e4d4455fd21f96582ffcbb0fd3d225ac0805a84dbc35529cf095432`.
The corrected binary was built before commit and implements `6c68806`, SHA-256
`e7e7de54c62afda766d42c5624e7b203d0b1de2561b1a063d44fb1c16578b5a6`. It matches the final canonical verification binary.
All scans use `CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, `GOWORK=off`, and
`go vet -vettool=<immutable binary> -enable-all -json`.
The [ledger](imported-async-followup-2026-10-01.tsv) records pins and scopes.

| Repository | Scope | Result |
| --- | --- | --- |
| golang/debug | `./cmd/viewcore`, parent | Exit 0; reviewed `main.go:822:13` present. |
| golang/debug | Same scope, corrected with tracing | Exit 0; only finding difference is removal of that FP. |
| ozontech/cute | `./...`, corrected | Exit 0; reviewed resource leak at `test.go:614:13` retained. |

All scan stderr receipts are empty. The trace
`.build/goal-profile-writer-corrected.trace.jsonl` labels the profile call at
`main.go:827:24` as `call-effects-asynchronous-exposure/unknown`, then accepts
`opaque-consumption` at the acquisition expression `main.go:822:22` (the same
expression as the original diagnostic's `822:13`). The local facts dump
`.build/goal-start-profile.facts.txt` preserves the already-exported async effect;
`.build/goal-profile-writer.trace.txt` records the parent rejection.
Viewcore is an affected-package scan; Cute is a complete repository scan.
No candidate tests, applications or generators were executed.

Final `make verify` passes, including all ordinary tests, formatter, vet, lint,
deadcode and local dogfood. A test-literal dupword lint issue in the first gate
was corrected before the final passing gate. No full precision corpus ran.
Beads `gohawk-dho.9` owns this correction. The remaining queue and broader
consolidation objective remain active.
