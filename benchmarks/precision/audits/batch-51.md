# Batch 51: source review and bounded corrections

The original 250-repository scan uses source `f105aa55db1f4bd40893f459cde2e0a676276ba9`.
The original binary is preserved separately from all corrected replays. Selection,
scan failures and source-review verdicts are recorded in the companion ledgers;
an incomplete scan is not a clean repository. Findings are static source-review
judgments, not runtime reproductions, and bounded cleanup-lifetime policy findings
are distinguished from demonstrated leaks or concurrent hazards.

## Verified corrections

All 421 original findings are reviewed: 325 true positives, 85 false positives
and 11 inconclusive. The selected-module scan completed for 193 repositories;
57 were incomplete. Of the 85 false positives, 24 have verified corrections
below and 61 remain unresolved. See the per-site correction receipt ledger.

Twenty-four original false-positive locations have pinned correction evidence:

| Evidence boundary | Locations | Change |
| --- | ---: | --- |
| Nested resource published through a global aggregate | 2 | Existing transfer uncertainty also covers global publication; scalar observations do not qualify. |
| Helper closes the actual merged resource argument | 4 | Every-return cleanup of a selected argument makes identity uncertain, not proven release of each acquisition. |
| Retained variadic aggregate contents | 1 | Reuse a positive argument-retention fact; no mock-framework exception. |
| Exact fresh parent context cancellation | 11 | Reuse the cancellation classifier for the direct parent as an alternative uncertain owner. |
| Early WaitGroup.Done without independent completion evidence | 3 | Decline readiness-only obligations instead of treating subsequent work as a defect. |
| Receive through a constructor-supplied aggregate channel | 1 | Existing containment establishes uncertainty, not an exact join. |
| Local worker context with dominating exact deferred cancellation | 1 | Decline the context-mode diagnostic; strict join mode still requires a join. |
| errors.As on the exact acquisition error | 1 | Its true branch proves acquisition failure; unrelated, joined and negative matches do not. |

These changes are committed separately as `c04189c`, `0182cad`, `f50a566` and
`c9609c6`. They refine existing decision points and reuse shared storage,
completion and lifecycle evidence. No analyzer was retired, disabled or demoted.
Two former merged-owner resource fixtures and three early-Done fixtures were
removed as explicit accepted coverage gaps. The historical round-10 early-Done
true-positive replay label was likewise removed, not relabeled false positive.

Round 55 records whole-repository replay labels only for fully scannable
repositories: all 21 labels pass, with ten corrected false positives absent and
eleven true-positive controls present across eight repositories. Four originally
incomplete repositories have scoped correction
evidence instead:

- Diun: all eleven reviewed cancellation sites disappear when scanning its ten
  notifier packages and `pkg/registry`; the exact parent-cancellation trace is
  unknown, not a claim that the child cancel function was called.
- KubePi: `internal/api/v1/proxy` no longer reports the reviewed worker at
  `proxy.go:386:3`. Alternative terminal Done paths are not newly proved joins;
  the readiness-only boundary declines this uncertain obligation.
- go-concurrency-patterns: the ring-buffer example's exact constructor channel
  receive makes the aggregate-mapped completion uncertain; its report disappears.
- BBGO: `pkg/cmd/exchangetest.go:238:2` is declined with reason
  `locally-canceled-context`. The worker receives the exact locally derived context,
  whose sibling cancel is deferred before launch. No timeout inference is needed.

The corrected full-repository cohort keeps nearby missing resource releases,
abandoned sends and the Authorizer signal-registration cleanup defect. Local
fixtures also retain mismatched owners, conditional cleanup, overwritten bodies,
ignored cancellation, strict joins and unrelated error guards.

## Check-level reassessment of unresolved families

The source-review ledger remains authoritative for each original verdict. An
unresolved false positive is not promoted to a passing regression label merely
because a possible fix has been described.

- **Correlated acquisition and helper arguments:** wg-portal passes the exact
  HTTP response/error tuple to helpers that close only on successful acquisition.
  Existing every-return completion lacks the paired-error assumption. A bounded
  relational contract is plausible but broader than a classifier adjustment.
  Five neighboring pfSense helpers genuinely leak on ReadAll failure before
  registering Close; merely finding a possible Close would hide those defects.
- **Retained or returned owners:** receiver maps, mutable constructor results,
  cleanup collections and returned control-channel fields can move ownership
  beyond the reporting function. Examples include Slackdump, Regclient, MATLAB,
  Shell Operator, Mysterium and Wingo. Reuse exact storage and retaining-effect
  evidence where available; collection-wide cleanup and receiver-return aliases
  remain unresolved. Do not substitute method names or framework conventions.
- **Transaction-owned rows:** five Indigo queries use statements prepared on a
  local transaction with unconditional deferred Rollback. The standard library
  propagates that transaction context to Rows and closes them on rollback.
  Extending the bounded SQL parent relationship is plausible, but must distinguish
  DB/Conn statements and mutable parents. Statement Close alone is insufficient.
- **Feasible paths and failed acquisition:** local Boolean release guards,
  correlated fallback files, mode checks, deterministic failing transports and
  table-test expectations require evidence not supplied by a generic missing
  Close rule. The exact errors.As implication is fixed; the remaining correlated
  or environment-specific paths are explicit gaps, not name-based exemptions.
- **Worker termination and indirect joins:** closing the exact connection can
  unblock a copy worker; retained channel maps, callback barriers and causal
  multi-worker joins can provide lifecycles outside a local receive. Current
  exact-context and aggregate boundaries cover only the verified shapes above.
  External cancellation without a visible terminal path remains inconclusive.
- **Producer cardinality:** Rootlesskit's target-PID reap, Mysterium's mutually
  exclusive success/failure sends and its bounded SSE test expose limits of
  counting syntactic sends. Keep these unresolved rather than adding PID, message
  count or project-specific rules. They require a general feasible-protocol proof.
- **Concurrency phase and value semantics:** Gauge's reverse lock order occurs
  before publication; Nanobot returns an already-allocated map whose contents
  are updated by decoding; Sloth retains output files for a later generation
  loop. These need phase, alias or later-use evidence, respectively. Adjacent
  real lock hazards, struct/nil-map return mutations and unnecessary loop defers
  remain useful controls.
- **Process and synthetic transport lifetimes:** intentional parent-exit handoff,
  child-only exit fixtures and in-memory/no-body HTTP transports can make local
  cleanup diagnoses misleading. Timed wrappers and external behavior were not
  guessed away; ambiguous cases retain an inconclusive verdict. No broad process
  exit or HTTP-response exemption was introduced.

These are retained checks with bounded improvements or explicitly unresolved
evidence requirements, not a conclusion that the broader questions are impossible.
General paired-argument, retained-owner and protocol models carry more complexity
than the small uncertainty changes validated in this batch.

## Validation scope

The canonical `make verify` gate passes after the final errors.As change:
generation, format/generated checks, module verification, vet, lint, deadcode,
self-dogfood, all package tests and shared-pass race checks. Full `go test -race
./...` passed before the last two bounded changes; changed goroutine and resource
packages are separately rerun under the race detector afterwards.
Both changed-package race reruns pass, including the final resource change.

Previously established rounds 53 and 54 pass their 24 and 17 labels. Their
20 and 8 disappeared baseline findings, respectively, were reproduced with the
original pre-fix binary and are historical drift, not new regressions. No new
baseline findings appeared in those replays.

The audit validates unique finding keys, exact revision pins, original check IDs,
complete per-finding review coverage, and source locations against pinned
checkouts. Neither scanning nor replay executes candidate tests, generators or
applications. This finding-based audit measures reviewed precision, not recall.
