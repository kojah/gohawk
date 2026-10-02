# Process startup result capability and helper retention

Bead: `gohawk-dho.23.19`. Parent: `2b87f6c`.

## Evidence and correction

The pinned Ferro trace submitted `configureProcGroup`'s void SSA result `()`
to wrapper cleanup queries. Startup discovery admitted every command-consuming
call and every tuple projection. Shared `heapmodel.CanHoldReference` now walks
SSA tuple components by value, alongside structs and arrays. Startup discovery
excludes void/scalar-only results and scalar projections. Reference-bearing
aggregate, pointer, interface and function results remain eligible. Capability
is a prerequisite, never an ownership guarantee.

A minimized conditional-Wait fixture exposed a second real precision gap:
a void helper stores the command in a global before Start. Filtering result
values must preserve the instruction's effects. For dominating helpers without
reference-bearing results, startup ownership uses the existing traced
`LifecycleEvidence.CallEffectsWithin` query. Possible retention, asynchronous
exposure and incomplete evidence leave ownership unknown. Known reads and
configuration writes do not transfer the obligation. The query shares the
candidate allowance and retains its own child cap; no new call-binding or SSA
walk was introduced.

## Controls and validation

Actual SSA controls exclude void calls, scalar calls, scalar-only tuples and
integer projections from mixed tuples. Pointer and aggregate results and the
reference projections of mixed tuples remain candidates. Typed tuple controls
also cover interfaces, functions and nested reference-bearing aggregates.
The helper controls enumerate cutoffs through a completed answer for retention,
configuration writes, unused scalar arguments, scalar tuples and unavailable
bodies. The fixture keeps a true uncovered conditional-Wait diagnostic beside
the accepted registry handoff. Fire-and-forget/PID-only patterns remain outside
the existing check; they were not used to manufacture diagnostic controls.

Focused heapmodel/process tests and `make verify VERIFY_TIMINGS=1` pass,
including generation, formatter, module verification, vet, lint, dead code,
ordinary tests and self-dogfood. Ordinary tests took 106 seconds and dogfood
56 seconds. No race run or full precision replay was performed.
Counterfactual overlays fail the tuple capability assertions, owner counts and
accepted registry fixture respectively. The correct fixture overlay runs
`TestProcessClosureChoices`; an earlier `TestAnalyzer` overlay only exercised
the separate processownership package and passed, so it is not credited as a
handoff regression check.

Complete all-check fixture payloads change process findings from 42 to 41:
only `processchoices/prestart_values.go:14:12` is removed. Producer payloads
remain identical at 22 findings. No diagnostics are added.
Receipts: `.build/goal-process-owner-values-fixtures/comparison.json`.

Frozen parent SHA256:
`88127c0832bf137c25cf9805679677a0fa8f87133904428967af4a98c7714e1f`.
Frozen current `.build/goal-process-owner-values-reviewed` SHA256:
`4bd77c458e07fffa0b34fa9564c6b03e5fc2d0dc01a26fd8ef61008aa9a506c0`.

## Remaining production sites

Five pinned clean scopes run with every check enabled, readonly modules and
GOWORK disabled; Rune requires CGO enabled, others use CGO disabled. All exit 0
with empty stderr. Full parent/current diagnostic payloads are identical:

| Repository | Pin | Scope | Findings | Recorded site status |
| --- | --- | --- | ---: | --- |
| Skywalking | `e83d5925500a7e63dd55c080a9b1542d6cedaefb` | `./pkg/tools/buffer` | 2 | Both read-lock/field-association alerts remain |
| Openase | `e530faf137e764337d5beaaf68af3be159eb17aa` | `./internal/infra/hook` | 0 | Two sites remain unknown with budget cutoffs |
| goiardi | `937cae400a92d8036b88ae2f65d93506271c292e` | `./shovey` | 9 | Both recorded caller-precondition alerts remain |
| Ferro | `d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4` | `./mcp` | 1 | Original process site absent with budget cutoff; one unrelated goroutine finding |
| Rune | `3e2165f8983280542c985947378dfa740a397d03` | `./internal/ide/idepkg` | 1 | Guarded fresh-lock publication alert remains |

Ferro's configuration helper now has known mutation, no retention and a real
command target. The next wrapper query still cuts off on a pipe-result tuple
`(io.WriteCloser, error)`, so production silence is not credited as a fix.
Constructor/receiver-state Start guarantees remain missing. Openase needs
reader-to-session completion evidence; goiardi needs caller-precondition and
mutable-state stability; the lock cases need their field/publication relations.
No target-resolution-only fix has been demonstrated for these sites.

Receipts: `.build/goal-process-owner-values-pins/scans.json`, `comparison.json`
and `ferro-target-events.json`. The preceding current-only reassessment is
`.build/goal-seven-current/scans.json` (Bead `.23.18`). Frozen historical labels
were not changed. Seven production sites plus Rune and the broader semantic
consolidation review remain unresolved; goal completion is unproven.
