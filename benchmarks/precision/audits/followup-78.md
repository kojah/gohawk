# Follow-up of the 78 remaining non-resource false positives

This pass follows the [resource tightening](resource-tightening-2026-09-22.md)
and reassesses all 78 non-resource findings still reported in the
[337-site review](followup-337.md). Original review labels are preserved.
Eleven further false positives are corrected; 67 remain unresolved.

| Analyzer | Input | Corrected | Still reported |
| --- | ---: | ---: | ---: |
| lockorder | 25 | 2 | 23 |
| goroutineownership | 24 | 7 | 17 |
| processownership | 6 | 2 | 4 |
| cancellationownership | 5 | 0 | 5 |
| producerlifecycle | 7 | 0 | 7 |
| concurrentcapture | 5 | 0 | 5 |
| deferinloop | 4 | 0 | 4 |
| evalorder | 1 | 0 | 1 |
| channelsafety | 1 | 0 | 1 |
| **Total** | **78** | **11** | **67** |

Together with the 33 resource corrections, this reduces the previously
reported 251 unresolved false positives to **207: 140 resource and 67 other
findings**. It does not complete their resolution or reclassify the remaining
reports as acceptable noise. The previously inconclusive resource site remains
inconclusive despite its diagnostic disappearing.

## Landed changes

- `4bf1a39`: a merged process `Wait` receiver can establish uncertainty without
  claiming the correct command was reaped.
- `d21bfcd`: lock flow retains its immediate predecessor for existing shared
  branch feasibility; proven local mutex allocations retain instance identity.
- `6e226c1`: goroutine ownership preserves factory copies, aggregate and
  type-asserted-owner handoffs, and exact locally canceled contexts passed to
  opaque helpers. A terminal `WaitGroup.Done` can precede only nonblocking
  completion defers, not arbitrary remaining work.

These changes reuse existing storage, flow, containment, and lifecycle
machinery. No check was removed, disabled, or demoted; no scheduling engine,
new shared summary family, or project-name exception was introduced.

The three landed slices retain all 67 of their real-bug controls: 56 goroutine,
four lock, and seven process/cancellation findings. The goroutine replay also
preserves all 16 earlier false-positive corrections. These controls constrain
regressions; they do not measure general recall. Each slice documents its
intentional uncertainty and possible coverage loss.

## Rejected approach

Requiring complete whole-worker summaries in the producer count proof removed
the seven service-lifetime false positives, but lost all 26 producer bugs still
detected in its control population. That experiment was rejected and every
producer implementation, fixture, and documentation change restored. It is
recorded separately, not counted among the eleven corrections. A smaller
service-return or ownership boundary remains future work.

The remaining families need evidence beyond these changes: transport/view
shutdown relationships, caller-held lock contracts, publication and guard
correlations, process lifetimes, exact collection cardinality, or specific API
semantics. The assessments below identify the gaps rather than adding one
exception per finding.

## Evidence and validation

- [Lock assessment](followup-78-locks.md) and [ledger](followup-78-locks.tsv).
- [Goroutine assessment](followup-78-goroutines.md) and [ledger](followup-78-goroutines.tsv).
- [Process/cancellation assessment](followup-78-process-cancel.md) and [ledger](followup-78-process-cancel.tsv).
- [Miscellaneous assessment and rejected experiment](followup-78-misc.md) and [ledger](followup-78-misc.tsv).

The ledgers keep original verdicts, assigned sites, controls, source pins,
binary hashes, and replay outcomes distinct. Corrected sites and their controls
are verified with the direct all-enabled CLI including test source. External
tests, applications, and generators are never executed. Package-scope success
is not a claim that every module in a repository loads.

Final combined `make verify` passes. Focused race tests pass for every changed
analyzer and the changed lifecycle-facts pass. Canonical round 58 still passes
70 runnable labels (60 false positives absent and ten true positives present);
the same 25 labels in six unscannable repositories remain excluded, not passed.
Receipts: `.build/followup78-final-verify.log` and
`.build/followup78-final-round58.log`.

The resource pass has one separately documented new false negative, piko's
unclosed WebSocket client. It is not hidden by the zero measured control losses
for the later non-resource changes.
