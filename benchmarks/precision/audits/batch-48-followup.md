# Batch 48: bounded fixes and remaining model decisions

The first two families were addressed in `049d8c9`, with inferred timer-owner
obligations removed consistently in `ca55b32`. The local `make verify` gate
passed. The initial round-50 replay retained seven sampled FP suppressions and thirteen
resource true positives; scoped round-49 replays retained two lock FPs and
one resource FP, plus thirteen resource true positives. No candidate tests,
generators, or repository scripts were executed. This was not a cumulative
precision replay or a fresh scan of every batch-48 module.

## Implemented boundaries

- Channel `NewTimer`/`NewTicker` calls no longer imply a Stop obligation.
  Missing Stop alone does not prove a leak with Go 1.23+ GC semantics, and a
  package-local pass cannot establish legacy main-module/runtime settings.
  Inferred cleanup facts also exclude timer-only owners, so wrappers do not
  recreate the obligation. File-owning positive controls still infer cleanup.
  The change removes timer stop/drain special cases instead of adding them.
  Twenty-two old timer-only executable labels were retired explicitly.
- A visible pointer-receiver getter whose entire body returns the address
  of one direct field keeps that field's identity. Different owners and
  fields remain distinct. Branching, loading, allocating, side-effecting,
  and imported bodyless getters are opaque. The codex2api proxy corrections
  exercise that conservative imported-call boundary, not cross-package
  getter inference. Local fixtures exercise the positive identity proof.

## Failed acquisitions and parent cleanup

The nine originally outstanding MariaDB-operator findings are in the pinned bundled MySQL
driver tests, not the operator's production code. Source revision:
`e8ece7a8076954674e10e0381571bd80278ac35f`.

| Cases | Evidence | Recommended boundary |
| --- | --- | --- |
| `driver_test.go:2799` | Exact context cancel precedes PrepareContext. | A bounded, exact cancellation-before-acquisition contract is feasible. Do not infer it from a test name or an expected-error string. |
| `benchmark_test.go:385`, `driver_test.go:2809,2844` | Parent DB is closed by a defer or test-harness Cleanup. | Parent-child cleanup is modelable for documented SQL statement ownership. Require the exact parent and proven cleanup, or abstain when the harness is opaque. |
| `driver_test.go:2728,2748,2854,2923,2929` | Timed cancellation, cancellation inherited by query rows/transactions, or a driver connection expected to be unusable. | Do not model sleep durations, scheduler ordering, or MySQL failure behavior. An expected error is not proof that acquisition failed. Narrow/abstain where the acquisition or lifecycle cannot be established. |

The standard library supports two general contracts: `driverConn.finalClose`
closes its tracked prepared statements; `Rows.awaitDone` closes rows when the
query/transaction context ends. Neither licenses treating DB.Close as cleanup
for arbitrary outstanding resources or treating PrepareContext cancellation
as ownership of every returned statement forever.

The parent-cleanup benchmark has an explicit stmt.Close on the successful
path; the relevant early error return is protected by deferred DB.Close.
The other two statement cases get DB.Close through runTestsParallel's
testing.Cleanup. Proving that caller harness relationship is more involved
than recognizing a direct defer. Start with the direct exact-parent contract,
not a general framework interpreter.

### Implemented bounded follow-up

Round 51 now covers the direct parent defer at benchmark_test.go:385 and
the exact pre-canceled context at driver_test.go:2799. The former follows
DB.Prepare/PrepareContext only, with exact receiver identity or two loads from
a singly initialized local cell before any potentially mutating escape.
Parent close is an opaque lifecycle boundary, not proof of immediate Stmt
invalidation. The latter requires a dominating synchronous call to the cancel
returned alongside that exact WithCancel/WithCancelCause context, and applies
only to DB.PrepareContext, DB.QueryContext, and DB.BeginTx's entry check.

This expands two bounded standard-library contracts rather than adding a
framework interpreter. Local fixtures preserve warnings for different or
reassigned parents, conditional cleanup, asynchronous cleanup, and unrelated,
conditional, deferred, or late cancellation. Rows and transactions retain
their own cleanup obligations. Two harness false positives remain, and five
driver/timing cases were subsequently reclassified as inconclusive below;
this does not claim that the entire SQL family is fixed.

The full local make verify gate passes. Round 51 retains both corrected false
positives and the transaction-leak positive at driver_test.go:1955. External
validation is static analysis only; no candidate tests or scripts ran.

## Contradictory lock order: a real hazard

The formerly inconclusive finding is resolved as a true-positive hazard.
Revision: `4f96afe95bb16132347f4ab74e63b0b1fa0f778b` of codex2api.

- Forward: `auth/store.go:8637` holds Store.mu; EnabledGrokAccounts calls
  IsGrokAPI at line 8641, which takes Account.mu in `auth/grok_account.go:132`.
- The new `opposite-order-recorded` trace identifies
  `auth/scheduler_outbox_consumer.go:395` as the first stored reverse edge:
  Account.mu held through recomputeEffectiveAutoPause. That particular
  account is fresh, so this trace alone is insufficient to prove shared-lock
  contention. Trace instrumentation was committed in `6d58404`.
- The same call occurs under a **shared** account lock in
  applyPersistentAccountSnapshot (`scheduler_outbox_consumer.go:464,573`),
  recomputeAllEffectiveAutoPause (`store.go:9053-9055`), and
  ApplyAccountQuotaAutoPauseConfig (`store.go:9266,9279`).
- recomputeEffectiveAutoPause calls resolveEffectiveThreshold
  (`store.go:1705-1750`), which can call GetGlobalAutoPause5h/7dThreshold.
  Those getters take Store.mu (`store.go:8815-8826`). Thus the reverse order
  is Account.mu → Store.mu on shared accounts, not just on a fresh object.

The warning also survives a static scan of production GoFiles only, ruling
out test-only contamination. Reader locks do not make the inversion harmless:
a pending Store writer can block a subsequent Store reader while an existing
Store reader waits for an Account writer. No runtime deadlock was executed;
the label claims a concrete ordering hazard, not a reproduced incident.

Keep this check for this case; no suppression is justified. The investigation
does not certify every class-level order report: fresh-object-only edges and
infeasible interprocedural branches still require conservative review. Trace
evidence now exposes both acquisition locations without changing diagnostics
or corrupting `-json`; traced and untraced codex2api JSON matched exactly.

The audit now has 62 defect/hazard positives, 23 policy-only positives,
79 false positives, five inconclusive locations, and 188 retired-check reports
after the SQL reassessment below. The lock hazard is round 50's fourteenth true-positive control.
The final round-50 replay on `ca55b32` passed all 21 labels: seven false
positives absent and fourteen true positives present, with both repositories
scannable and no baseline drift.

## SQL reassessment after round 51

The earlier cancellation verdicts conflated test intent with guaranteed
behavior. Findings at driver_test.go:2728,2748,2854,2923,2929 are now
**inconclusive**, not false positives or confirmed leaks. The first, second,
third, and fifth depend on scheduled cancellation; the fourth expects a
previous operation to leave a driver connection unusable. Nonfatal Errorf does
not exclude success. These functions defer stopping the cancellation timer,
and Sleep does not join its callback. Conversely, lack of a package-local
proof does not establish a feasible driver success or runtime leak. No
suppression or new executable verdict was added for these five cases.

### Callback ownership investigation

The statements at driver_test.go:2809 and :2844 remain reviewed false positives.
The pinned harness at :186-241 registers cleanup of the DB it passes to each
callback, in both subtest variants. An actual SSA dump shows the extra work
needed to connect those facts:

1. Each test stores its callback in a variadic slice passed to the harness.
2. The harness loads each slice element, stores it in a local, and captures
   that local in each subtest closure.
3. The subtest registers a cleanup closure for its fresh DB, stores the same
   DB in a wrapper field, then dynamically invokes the loaded callback with
   that wrapper.
4. The callback loads that field and prepares the statement.

The existing StaticCallsites index deliberately excludes calls with no static
callee. cleanupRegisteredBefore examines the acquisition's own function, not
the caller harness. Lifecycle facts describe individual parameters, not the
relationship between a callback's parameter field and a resource created by
its caller. A new Boolean fact such as Invoked cannot express this ownership.

A general solution is possible, but is not a small extension of the direct
parent-close rule. It needs bounded callback-value propagation through slices,
stores and closure bindings, argument-to-parameter field substitution, and
cleanup coverage for every reachable callback invocation. Unknown targets,
mutation, mixed parents, conditional registration, and escaping callbacks
must terminate the proof conservatively. The local traversal helpers remain
useful mechanics, but do not themselves supply that relational evidence.

Decision: retain the check and the two known precision gaps for now rather
than add a harness-specific recognizer or silently exempt every statement
created through a caller-owned DB. The latter would also hide ordinary leaks
against long-lived pools. A broader callback-ownership implementation should
be a separately scoped change, justified by additional instances of the same
contract. No analyzer behavior changed in this reassessment.

Validation: pinned source and SSA inspection, ledger/count consistency, and
round-51 static replay. No candidate tests, generators, or scripts executed.
