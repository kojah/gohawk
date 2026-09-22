# Follow-up of the 337 unresolved false-positive locations

This follow-up reviews the fixed set in [the input ledger](followup-337-input.tsv),
not another repository sample. Original batch verdicts remain unchanged;
subsequent corrections and revised judgments are recorded separately.

## Result

All **337 sites were re-reviewed**. The [per-site results](followup-337-results.tsv)
retain their exact repository revisions, checks, evidence and replay receipts.

- **82 newly corrected false-positive sites.**
- **3 sites already absent at the starting revision**, not credited to these changes.
- **251 still reported**, with the missing evidence documented individually.
- **1 original false-positive judgment corrected to inconclusive.**

Thus 85 of the original locations are now absent; this is not a claim that
all 337 were fixed, or that the complete bug families are covered.

Detailed reassessments: [resource lifetimes](followup-337-resources.md),
[worker lifecycles](followup-337-workers.md), and
[locks and other checks](followup-337-locks.md).

| Analyzer | Reviewed | Absent | Still reported | Review correction |
| --- | ---: | ---: | ---: | ---: |
| cancellationownership | 9 | 4 | 5 | 0 |
| channelsafety | 1 | 0 | 1 | 0 |
| concurrentcapture | 5 | 0 | 5 | 0 |
| deferinloop | 5 | 1 | 4 | 0 |
| evalorder | 1 | 0 | 1 | 0 |
| goroutineownership | 40 | 16 | 24 | 0 |
| lockorder | 33 | 8 | 25 | 0 |
| oncepolicy | 1 | 1 | 0 | 0 |
| processownership | 11 | 5 | 6 | 0 |
| producerlifecycle | 21 | 14 | 7 | 0 |
| resourcelifetime | 210 | 36 | 173 | 1 |
| **Total** | **337** | **85** | **251** | **1** |

The three already-absent sites are resource reports; their rows explicitly
separate baseline absence from a newly verified correction.

An initially counted fourth baseline absence, geesefs' logger acquisition,
did not hold in the canonical all-checks replay. It remains unresolved rather
than being credited from an isolated replay profile.

## Baseline and scope

- Baseline source: `c9b65a6`.
- Baseline binary SHA-256:
  `33743c44dfe1f68cbe8dc146989dbe76c0ab02bda55fa8f030a10e32861dfa02`.
- Original repository pins and finding keys are retained in the input ledger.
- Candidate execution is limited to static analysis. No candidate tests,
  generators, or applications are run.
- Failed package loads, timeouts, and missing analysis output cannot establish
  that a finding was corrected.

| Analyzer | Locations |
| --- | ---: |
| resourcelifetime | 210 |
| goroutineownership | 40 |
| lockorder | 33 |
| producerlifecycle | 21 |
| processownership | 11 |
| cancellationownership | 9 |
| concurrentcapture | 5 |
| deferinloop | 5 |
| evalorder | 1 |
| channelsafety | 1 |
| oncepolicy | 1 |
| **Total** | **337** |

## Review method

The review separates bounded fixes, conservative uncertainty boundaries,
remaining evidence gaps, and corrections to the original review. An absent
diagnostic is not proof that the candidate program is correct.

Initial resource corrections cover exact error/success relationships:

- A helper consuming both the acquired resource and its exact error, with
  visible possible cleanup, makes the ownership proof unknown. This does not
  add conditional summaries or claim the helper always releases the resource.
  Helpers with no cleanup witness or a different error remain diagnostic.
- Equality of the exact acquisition error to a documented non-nil filesystem
  sentinel establishes an acquisition-failure branch. An arbitrary error
  variable, unrelated error, or inequality does not establish that evidence.

The pinned wg-portal, ch.at, and enc scopes replay successfully with these
changes: eleven original false-positive locations disappear, while ch.at's
two other unclosed-response findings remain.

No analyzer or check has been retired, disabled, or demoted. Remaining reports
are recorded as evidence gaps, not relabelled to make a precision gate pass.

## Implementation

| Commit | Bounded change |
| --- | --- |
| `31ef96c` | Shared lifecycle summaries require a positive action witness and a reachable normal return. |
| `a1d23d2` | Goroutine ownership uncertainty for factory signals, aggregate projections, ongoing worker loops, and exact local cancellation. |
| `c6b3e6f` | Producer checks decline unproven loop cardinality and callers without normal returns. |
| `18cb072` | Process receiver identity and opaque waiter handoffs. |
| `4d7bf27` | Resolve captured parent contexts at child creation. |
| `eaa54b3` | Lock callback uncertainty, deferred acquisition timing, fresh lock modes, and external writer guards. |
| `8fa11ea` | Direct package initializers are already evaluated once. |
| `3dc8539` | Aggregate storage can imply collective, rather than per-iteration, resource lifetime. |
| `0806f8d` | Exact error/resource pairs, transaction-owned rows, possible asynchronous cleanup, and aggregate retention. |
| `5a56fb4` | Exclude direct resource views from aggregate-retention uncertainty. |

The shared-summary fix matters beyond the motivating resource reports. A
panic-only method had been exported as releasing every field and completing
every parameter obligation, because no normal return contradicted those
claims. Completion summaries now require actual completion evidence;
non-returning functions do not establish arbitrary lifecycle contracts.

The resource handoff rule was narrowed during validation. A returned value
derived from resource data is not necessarily an owner of that resource.
JSON transformation, cookies extracted from a response, and `bufio.Scanner`
are diagnostic controls; merely consuming bytes does not discharge `Close`.
The accepted uncertainty boundary requires an actual aggregate carrying the
resource and a bounded possible-retention relationship.

## Review correction

Piko's `server/proxy/server_test.go:406:12` was originally labelled false
positive because the test calls a local port “unreachable.” Static source
does not establish that no other service can listen there. The follow-up
records this as inconclusive, preserving the original label in the input
ledger. It is not counted as a verified fix.

## Coverage tradeoffs

Several corrections deliberately exchange recall for precision. An unlock
callback handoff makes held-lock state uncertain; it does not prove that an
unlock ran. Possible asynchronous cleanup does not prove synchronous cleanup.
A loop does not establish an exact producer count. Opaque ownership is not
promoted to a guaranteed cleanup or worker-completion summary.

There is a measured recall cost outside the 337 false-positive input sites:
five genuine wg-portal leaks at `internal/lowlevel/pfsense.go:332:60`,
`:355:58`, `:378:58`, `:401:58`, and `:424:54` are now missed. Their helper
calls `io.ReadAll` and can return on a read error before registering the
`Body.Close` defer. The paired-error uncertainty rule declines the caller
report because it cannot express the helper's conditional cleanup contract.
These remain real bugs and are recorded as accepted false negatives, not
corrected false positives. The original audit labels are preserved. Of 72
original resource true-positive controls in the replayed scopes, 67 remain
reported and these five do not.

The historical cohort comparison measured one additional accepted false
negative introduced here: kubernaut's
`docs/spikes/multi-cluster-mcp-gateway/spike-s15-fmc-multiformat-parse/e2e_spike_test.go:35:12`.
It hands a callback capturing the command to `t.Cleanup`, but that callback
only calls `Process.Kill`, not `Wait`. Killing does not reap the process.
The opaque callback handoff now makes ownership unknown, so this real bug
is no longer reported. Baseline and final pinned package loads both succeeded;
the original true-positive label remains unchanged. This makes **six measured
true-positive losses**, not six fixes.

Remaining sites need stronger evidence about branch correlations, conditional
helper contracts, publication and participant identity, transport shutdown,
or typed lifecycle protocols. No project-name exceptions, generic `main`
exemptions, suppression comments, or scheduling state enumeration were added.

## Validation and reproducibility

The final analyzer implementation revision is `5a56fb4` (ten focused
implementation commits). The authoritative final executable for all three
replay groups has SHA-256:
`3da9ce166dfd8dc72c3225dc1229298040ae27b04a394c49b2edcf7c3ca772a7`.
Earlier resource replay binaries remain intermediate evidence only; all 104
resource scopes were repeated with this exact executable after the geesefs
profile discrepancy was discovered.

All **192 package/analyzer scopes** loaded successfully: 104 resource, 55
worker, and 33 lock/miscellaneous scopes. The final worker and lock diagnostic
sets exactly match their preceding verified replay. Source pins, selected
checks, binary hashes, commands and positions are retained in the per-site
ledger and its receipts. Final scoped replays explicitly select input check
IDs. Earlier lock replays omitting extended checks were rejected as evidence
and replaced with explicit-check replays.

`make verify` passes on the final source: formatting, generated-code checks,
vet, lint, dogfood, repository tests, and the standard shared-pass race gates.
The final resource analyzer race test also passes. A full `go test -race ./...`
attempt is **not clean**: it encountered the previously reproduced Go 1.27 /
`x/tools` package-loader race in the architecture tests. Resource fixture
failures during intermediate, changing worktrees are superseded by the frozen
final normal and race test runs; they are not recorded as final passes.

The [new regression cohort](../round-58/README.md) carries 85 absence labels
and ten real-bug controls. Whole-repository loading limitations and historical
cohort failures are reported separately from the successful scoped replay;
this audit does not claim a clean cumulative historical precision gate.

Round 58's final canonical replay succeeds for **70 checked labels**:
60 false positives remain absent and ten true positives remain present.
Another **25 labels are excluded** by incomplete whole-repository loading in
ch.at, wg-portal, httptap, wakapi, sonic and nelm. Their individual package
replays succeed, but they do not count as whole-repository gate passes.

The initial stamping run exposed a separate audit-tool bug: absent FP labels
in unloadable repositories were stamped despite being excluded from the
reported counts. Commit `eff5c6b` fixes this; a regression test fails before
the change and all 15 Python audit tests pass afterward. The 25 unsupported
whole-repository stamps were cleared. Original historical labels were not
rewritten.

[Historical gate comparisons](followup-337-historical-validation.md) cover
completed rounds 2, 3, 4, 5, 6, 8, 10 and 12. Across loadable scopes, all 84
previous FP labels remain absent; 75 of 94 TP labels remain present. Of the
19 missing TPs, 18 predate this work and the kubernaut callback case is the
new accepted false negative described above. Round 10 also has a pre-existing
label-count mismatch. Nine findings newly appearing relative to old censuses
were reproduced unchanged with the frozen baseline. Interrupted and unrun
rounds are disclosed in the comparison report, not counted as passing.
