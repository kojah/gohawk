# Returned-result observation allowance

Bead: `gohawk-dho.44.11.5.27.25`. Parent: `079848a`.

## Confirmed gap and shared correction

The lock return-contract walk already carried a request allowance, but
`successfulReturn` and `heldResultPolarity` resolved deferred result cells
through `lifecycle.ReturnedResult`, which created observation-time storage with
a separate nil allowance. `callerReleasesOnFlag` also selected call results
through the default `ssaflow.CallResult` metadata query. These inner queries did
not share the surrounding request's work ownership.

Real-SSA zero-allowance controls against the parent fail for deferred true,
deferred false and direct true returns: the result question completes without
marking exhaustion. Current controls require exhaustion at zero, prevent a
success answer at cutoff and preserve complete true/false policy.

One `lifecycle.ReturnedResultWithin` implementation now selects the result and
passes its allowance to observation-time storage. Unavailable storage retains
the original load; local or pool cutoff supplies no value. Nil allowance
retains default query semantics. All discovered production callers pass their
existing allowance: the two lock contracts, lock fresh-constructor evidence and
the goroutine cleanup factory's callback and sibling result reads. The old
unbounded wrapper had no remaining production consumers and was removed; its
test callers use the one shared implementation with nil allowance.

The caller-release result selector delegates `CallResultWithin` to the same
request. An interrupted Boolean result cannot masquerade as success, and an
interrupted caller contract cannot suppress a missing-release finding as safe.
The enclosing walk continues to discard staged diagnostics and order edges on
cutoff. Goroutine possible cleanup still supplies ownership uncertainty rather
than exact completion. No observation-time, must/may, caller completeness,
notification, identity or publication rule changes.

## Controls and compatibility

Shared lifecycle controls sweep every allowance through first completion for
deferred result cells, direct constants and opaque global loads. They compare
SSA value identity with default results, retain raw opaque loads, return no
value on local cutoff and recover with a fresh child without exhausting the
outer pool. Existing lifecycle deferred error-result controls and the full
lock/goroutine test packages pass. The helper index was regenerated, with no
new evidence engine or duplicated result traversal.

The first verification run passed ordinary tests but lint rejected the new
sweep harness's cognitive complexity. Splitting the sweep into a focused helper
resolved it. Final `make verify VERIFY_TIMINGS=1` passes all eight gates:
generate 2s, modules 1s, vet 1s, formatting 3s, dogfood 3s, deadcode 5s,
lint 5s, tests 8s.

Ten parent/current all-check scan invocations exit 0 with empty stderr.
Complete JSON payloads match for lock fixtures (116 findings), goroutine
fixtures (128), Rune (0), Skywalking (0) and stargz (1). Pinned package scopes
and revisions are retained in `.build/goal-return-result-scans/scans.json`.
Stargz's reviewed `store/manager.go:193:2` true positive remains. External
checkouts were verified clean and pinned; scans used read-only static vet,
CGO enabled for Rune and disabled elsewhere. No candidate tests, applications
or generators were run.

Reviewed/canonical binary SHA-256:
`2dffcbc4fca6730695d6c2edab8accf244ddc8e9e5a28410e254ce080742c7fb`.
Parent binary SHA-256:
`0d85fbba43136122fc2f0b0044295c23aca381ee21fa60bc0f336c17e6b05453`.
Local evidence uses `.build/goal-return-result-*` and
`.build/goal-lock-return-storage-parent.log`.

## Remaining finite scope

Source review found separate request-cost questions in lock
`copiedFieldLockIdentity` (storage created without its reaching walk allowance),
`possibleWriterAt` (default temporal queries and a repeated call-list walk), and
`exclusiveCallers.parameterExclusive` (all-caller exclusivity queries). These
remain in the open lock/identity parent; their polarity and graph ownership need
assessment before changing them. The package caller census already has one
whole-package allowance shared by conditional release and exclusivity. That
fact does not establish request ownership of the individual exclusivity queries.

Graph/index MCP tools were unavailable; this is bounded exact-source fallback.
No full precision replay, local race run, production FP credit, transitive
wall-clock completeness or overall architecture completion is claimed. The
five recorded production sites remain unresolved; the parent remains active.
