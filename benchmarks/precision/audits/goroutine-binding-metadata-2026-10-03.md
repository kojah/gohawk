# Goroutine binding metadata consolidation

Bead `gohawk-dho.44.11.7`; parent source `99f68c5`.

## Evidence and change

Exact source review found eager `CallBindings` collection before supplied
allowances in exact completion mapping, deferred completion groups, helper
join/escape searches and worker receive search. These consumers now enumerate
through the existing `CallBindingsWithin` iterator. Exact completion mapping
selects captures before arguments, without first preparing unrelated argument
metadata. The other consumers retain argument-before-capture order. Exact
identity, mutable-capture stability, aggregate ownership, must-complete versus
possible consumption, and owner versus worker completion retain their policies.

Escape cutoff remains opaque, receive cutoff remains unknown, and partial
discovery cannot publish an obligation through the constructor's availability
fence. The shared iterator owns metadata charging; consumers own the meaning
of incomplete evidence. No numeric allowance or schema changes.

`completion_metadata_test.go` compiles an actual SSA closure with one captured
aggregate and 64 unrelated arguments. A fixed allowance selects the capture
without exhaustion; a sweep checks that cutoff cannot publish a binding. The
parent overlay fails the selected-capture assertion with exhaustion. The SSA
dump confirms the captured allocation, free variable and 64 evaluated arguments.
Existing helper, receive, completion-binding and discovery controls pass,
including incomplete memo rejection and fresh recovery.

## Validation and scope

Final canonical `make verify VERIFY_TIMINGS=1` passes all eight gates. Six
read-only parent/current all-check scans preserve complete diagnostic payloads:

| Scope | Pin | Findings |
|---|---|---:|
| Goroutine ownership and closure choices fixtures | Current authored fixtures | 128 |
| Openase internal/infra/hook | e530faf137e764337d5beaaf68af3be159eb17aa | 0 |
| stargz store | 624678b4e421947534cbf0618f9609853cccee0f | 1 |

All scan exits are zero with empty stderr. Production checkouts are clean at
these pins, with readonly module loading. The reviewed binary matches the
canonical build: `c7c1261f3a15a200f3ba596e7ae1d4acebd214bbfce0f2a720aead4e2e70685d`.
Local ignored receipts are `.build/goal-goroutine-binding-scans/`, the focused
and canonical logs, parent overlay control, and actual SSA dump.

Graph/index tools were unavailable. The normalized review-aid scans still cover
334 production files and 2,202 declarations: five whole-body and 34 partial
candidate groups, with unchanged path/symbol/token signatures. This is not
proof of absence of differently structured semantic duplication. Standalone
policy traversals and graph, alias, type, allocation and rendering costs remain
separate review scope; no whole-query wall-clock bound is claimed. No full
precision replay, local race run or production FP correction credit is claimed.
