# Mutex identity allowance and receiver proof

Bead: `gohawk-dho.44.11.5.27.27`. Parent source: `81e6e52`.

## Confirmed gap and consolidation

Setup and bound effects had request allowances, but instance identity started
independent reaching and observation-time storage queries. Concrete receiver
selection also resolved identity in its leaf, agreement key and final action.
The parent consumer control completed a bound global-lock operation with two
visits without exhausting its allowance; identity work was hidden.

`lockIdentityWithin` shares the supplied allowance with reaching and storage.
Cutoff returns no identity before slot-name fallback. Receiver selection carries
one receiver/identity result through the shared fold. Setup, summary binding,
callee census, acquisition metadata and existing flow consumers use their
request allowances. Interrupted direct effects and summaries publish no partial
witnesses. Acquisition metadata retains the original receiver expression's
identity; it does not replace that attribution with the selected leaf.

Declaration-class widening moved intact to `classes.go`, separating its fresh
field and global-class policy from instance identity. Type, heap graph, alias,
resource-path and class-query costs retain their separate owners. Default
release-attempt presentation also remains outside the proof allowance.

## Controls and compatibility

`TestBoundMutexIdentitySharesOperationAllowance` reproduces the meaningful
parent failure. The first attempted parent control used a nonexistent fixture
resource type; that compile failure supplies no evidence. The corrected
actual-SSA consumer fails on the parent and passes after the change.
`TestMutexIdentityAndReceiverAllowances` covers global, field, copied receiver,
explicit snapshot, getter, constant index, agreed interface phi and mixed phi.
It sweeps allowances through first completion, rejects partial actions and
identities, checks fresh-child recovery and preserves exact completed receivers.

Ten read-only parent/current all-check invocations exit zero with empty stderr.
Complete diagnostic JSON matches for lock fixtures (116), goroutine fixtures
(128), Rune (0), Skywalking (0), and stargz (1). Production scopes use clean
pinned checkouts and read-only modules; Rune enables CGO. No external tests,
applications or generators were run. The comparison receipts retain pins,
scopes and binary hashes in `.build/goal-mutex-identity-scans`.

Reviewed/canonical SHA-256:
`fc6724eeb2c47d47a507453f29b38dd2700e3cc8e45f4402ea2fb6fca9ca6828`.
Parent SHA-256:
`5ea95b0ad5c0b602244b1b8a93dd0d8a751a56bca29e1c240c8d13547aedcf43`.

## Limits

Graph tools were unavailable; evidence uses exact source fallback and actual
SSA. The initial canonical run failed on an unused import after extraction;
the next failed on complexity and line length while ordinary tests passed.
Those defects were corrected without raising thresholds. Final
`make verify VERIFY_TIMINGS=1` passes all eight gates: generate 2s, modules
1s, vet 3s, deadcode 9s, formatting 10s, lint 14s, dogfood 31s and tests 57s. No full precision replay, local
race run, trace compatibility claim or FP credit is made. All-caller
exclusivity, broader consolidation and five production FP sites remain open.
