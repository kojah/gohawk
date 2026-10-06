# Read-lock receiver fields with opaque caller contracts

Bead: `gohawk-dho.23.23`. Parent: `1f3248c`.

The parent reports Skywalking cursor assignments from same-owner matching,
without establishing which field the read lock guards. Its
`ResetForLoopReading` method writes both fields without carrying an acquisition
or calling other code. The caller's synchronization or ownership contract is
opaque to this proof. A read/modify/write distinction was rejected because it
would still not identify a field's guard.

The existing read-lock decision now consumes one bounded package index of exact
receiver-field declarations written in complete call-free method bodies.
Those fields return `field-guard-unknown`. This follows the precision failure
ladder by widening unknown, without a name exemption, framework contract,
check demotion or a new proof file. It does not prove setters safe, infer guards,
or establish confinement. A setter always called under the same mutex can hide
a real read-lock write; that false negative is explicitly recorded.

The census uses shared instruction traversal and exact embedded-field paths.
Loaded peers and other parameters do not supply witnesses. A call, defer or
launch anywhere in the method disqualifies all its staged fields, including a
call after a store. Field declaration identity keeps siblings and different
types separate. Cutoff invalidates the index; partial negative evidence cannot
permit a read-lock report. The package index is collected once, while order and
release proofs retain their existing budgets. The flow object now carries the
shared inputs rather than extending a positional helper beyond eight arguments.

Graph/index/coverage tools were unavailable. Evidence is bounded exact source,
compiled SSA and affected-scope behavior; no broad semantic completeness claim
is made.

## Controls

The new `readlockfields` fixture fails on the parent with three unexpected
reports: two pointer cursor assignments and one scalar cursor update. Existing
SSA dumps show the actual field stores and distinguish the guarded reset's
calls. The corrected fixture retains four diagnostic controls for unrelated
fields, a write-locked reset and same-spelled fields on another type.

Actual SSA unit controls cover declaration identity, loaded-peer exclusion,
a call after a store, cutoff and fresh-allowance recovery. Counterfactual source
overlays accepting late calls or loaded roots both fail assertions, not builds.
Tracing tests assert the unknown reason and candidate association through the
same authoritative proof as reporting.

Focused lockorder and architecture tests and corrected lint pass. The initial
focused and canonical checks caught reason-test, argument-count, modern iterator,
Boolean-style, public-prose and rationale-comment issues; these were corrected.
The preliminary scoped batch overlapped a rebuild and is superseded. Neither
its binary attribution nor the failed canonical attempt is a completion receipt.
Authoritative final receipts use a separate immutable binary and output folder.

No full precision replay or local race run is part of this iteration.


## Final authoritative receipts

The final reviewed binary SHA-256 is
`9615a352193007fa4f9ed3a02389ce7cde95288b61b9e8cce483d4603cc7ac3d`.
The parent SHA-256 is
`f459687bdeab6f8e04735f875880472eed76f07fddb31bd572b5993a0ad69fd5`.
Twelve terminal all-check scans exit 0 with empty stderr. All current receipt
hashes match the immutable reviewed binary. Complete merged payloads have no
added findings and exactly these removals:

| Scope | Parent | Current | Removed |
| --- | ---: | ---: | ---: |
| lockorder + ordercycles + readlockpaths | 117 | 117 | 0 |
| privateread | 6 | 6 | 0 |
| readlockfields | 7 | 4 | 3 fixture FPs |
| Skywalking `./pkg/tools/buffer` | 2 | 0 | 2 reviewed production FPs |
| Rune `./internal/ide/idepkg` | 1 | 1 | 0 |
| goiardi `./shovey` | 9 | 9 | 0 |

Unchanged scopes preserve complete diagnostic payloads. Skywalking pin:
`e83d5925500a7e63dd55c080a9b1542d6cedaefb`.
Rune pin: `3e2165f8983280542c985947378dfa740a397d03`, CGO enabled.
goiardi pin: `937cae400a92d8036b88ae2f65d93506271c292e`.
All candidate checkouts were verified clean and scans use readonly module mode.

The final fixture traced invocation exits 0 with empty stderr, preserves its
untraced diagnostic payload and attributes `field-guard-unknown` to exactly
three accepted source positions. The final Skywalking traced invocation exits
0 with empty stderr and empty diagnostic JSON. Both original candidates have
`field-guard-unknown` decisions and no candidate budget-exhaustion events.

Final `make verify VERIFY_TIMINGS=1` exits 0: generation, module verification,
vet, formatting, lint, deadcode, self-dogfood (39 seconds) and ordinary tests
(76 seconds) all pass. Its binary hash matches the reviewed binary.
Local receipts: `.build/goal-readlock-fields-final-scans/{scans,comparison}.json`.
Canonical log: `.build/goal-readlock-fields-final-verify.log`.
Traces: `.build/goal-readlock-fields-{final,sky-final}.trace.jsonl`.

Two production sites are corrected. Six residual locations remain unresolved:
Openase two, goiardi two, Ferro one and Rune one. Openase/Ferro silence still
includes budget uncertainty and receives no FP credit. These scoped receipts
do not replace a full corpus census or prove broad architectural completion.

After the final documentation and completion-matrix updates,
`go test ./internal/testsupport/architecture` exits 0 (2.526 seconds).
