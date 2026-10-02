# Writer-release call census

Beads `gohawk-dho.23.1`.

## Reviewed duplication and correction

`possibleWriterAt` rediscovered every synchronous call in a function for each
candidate mutation and deferred writer witness. `buildLockSetup` already owns
the same ordered census, built once with `InstructionsWithin` and rejected in
its entirety at cutoff. The sole reporting consumer receives this completed
setup through `lockStateWalk.run` and `lockFlowContext.setup`.

The proof now takes that existing call slice. It still checks the deferred
writer's dominance, standard exclusive release identity, possible receiver
alias and feasible instruction ordering. Read unlocks, unrelated receivers,
known empty wrappers and deferred unlocks retain their distinct meanings.
No summary, acquisition, release or guard-to-field inference changes. Temporal
and heap-query internals keep their existing independent cost boundaries; this
is discovery consolidation, not a whole-query budget claim.

## Validation and limits

Existing `opaque_writer.go` regression fixtures pin held imported writers,
unrelated calls, known empty wrappers, writers after the mutation and an
explicit intervening release. Actual SSA for `earlyWriterReleaseBeforeWrite`
is `.build/goal-writer-census-release.ssa`: a synchronous standard Unlock
follows registration of the deferred Unlock and precedes the owner-field Store.
The completed setup census includes this call, so the diagnostic remains.
The full focused lockorder suite passes (`.build/goal-writer-census-focused.log`).

Four parent/current all-check scan receipts terminate successfully with empty
stderr (`.build/goal-writer-census-scoped/scans.json`). The lockorder and
readlockpaths fixture scopes retain all 113 diagnostic payloads exactly.
Clean pinned Skywalking `e83d5925500a7e63dd55c080a9b1542d6cedaefb`
`./pkg/tools/buffer` retains both distinct findings and identical payloads.
No new tests are added for this small inventory substitution; existing accepted
and diagnostic forms exercise the affected semantic boundary.

Parent binary SHA-256:
`6689a62a422c2e8e5b35ffc6017d52cd49e6cac0f3ccc88520aef8d24152560f`.
Current binary SHA-256:
`7b3a353a1faccabd2c8837c5eddbd5c100259e89efaa3f5c6456e9bcde9612d6`.
Final canonical `make verify VERIFY_TIMINGS=1` passes all eight targets
(`.build/goal-writer-census-verify.log`); final architecture validation passes
(`.build/goal-writer-census-architecture.log`). No full precision replay or
local race run is used. Graph tools are unavailable; source fallback verifies
the sole production call path and setup publication barrier.

No production FP removal is credited. Seven recorded production FP sites plus
Rune remain unresolved. This bounded correction supplies no exhaustive claim
about differently structured duplication or the overall architecture.
