# Producer fallback callee consolidation

Bead: `gohawk-dho.23.21.8`. Parent source: `bd6fec2`.

## Source decision and actual SSA

`producerSends` independently selected a static callee or literal closure.
`ssaflow.DirectCallee` already owns this selection, keeps dynamic dispatch
opaque, and resolves generic instances to their source origin. The distinction
is material: actual SSA for the generic control has an instantiation wrapper
that forwards to the origin, while the origin contains the send instructions.
The old fallback selected the wrapper and therefore collected no direct sends.

The fallback now uses the shared resolver before the existing
`SpawnedValueAtCall` binding. Local unbuffered-channel selection, ordering,
repeated-send uncertainty, count polarity, receiver rules and report gates are
unchanged. This consolidates callee-selection mechanics and restores generic
fallback evidence, rather than introducing callback resolution or changing
ownership policy.

`TestFallbackCalleeBindings` builds and logs real SSA for six shapes: ordinary,
captured, generic, balanced generic, dynamic function parameter and phi-selected
closure. It verifies source-body instruction identity, exact caller channel
binding and the final excess versus balanced count proof. The two generic
assertions fail against the parent implementation; the four other shapes pass.
Dynamic and selected workers still yield no candidates. The paired integrated
fixtures are `helpers/generic_fallback.go`.

## Validation

The full producer test package passes, including existing receiver budget,
ordering, unknown and trace controls. `make verify VERIFY_TIMINGS=1` passes all
eight gates: generate 2s, modules 0s, vet 0s, formatting 1s, deadcode 3s,
lint 4s, dogfood 20s, tests 51s. The public detection lead now includes generic
producer functions; detailed mechanics and controls stay in the design note.

Parent/current all-check scans preserve the 22 existing producer fixture
findings and 15 capture fixture findings. The only addition is the expected
`producerlifecycle/abandoned-send` diagnostic at `generic_fallback.go:21:2`.
This is a regression fixture, not a production FP correction. Complete JSON
payloads match after removing exactly that single, explicitly checked addition.

The producer traces retain all 216 existing records and add ten records:
four send candidate/decision pairs (three accepted, one rejected), and the
reporter's diagnostic candidate/decision pair. Capture retains 74 records.
Comparison verifies complete event contents and ordered sequences per candidate
after separately validating the expected new records; traced diagnostic JSON
matches the corresponding untraced scans. The initial comparator incorrectly
expected only eight proof records; the two additional reporter records were
inspected and the assertion corrected using the completed producer runs.

Reviewed/canonical binary SHA-256:
`0d85fbba43136122fc2f0b0044295c23aca381ee21fa60bc0f336c17e6b05453`.
Parent binary SHA-256:
`a067ea32d69d250859d1a1312059b93550d01ed80b60d3d2cb26c10dc348217e`.
Local receipts use `.build/goal-producer-callee-*`, including the parent-failing
SSA log, all-check scan receipts, expected diagnostic addition and full trace
comparisons.

## Limits

Codebase Memory tools were unavailable. This is an exact-source and real-SSA
review of the producer fallback, not a transitive absence claim for all callee
resolution consumers. No full precision replay, local race run, production FP
credit or overall goal completion is claimed. The wider semantic review and
five unresolved production sites remain open.
