# Deadlock-fix prevalence study (2026-09-24)

This targeted follow-up measures how often real, fixed Go deadlocks have the
shapes that gohawk's four experimental dependency-cycle checks target
(`lockorder/lock-and-join`, `lockorder/channel-lock-cycle`,
`lockorder/waitgroup-lock-cycle`, `channelsafety/dependency-cycle`). It
complements the [cycle-check recall audit](cycles-2026-09-24.md), where those
checks reported nothing on 100 pinned repositories.

## Method

`scripts/mine-race-fixes.py --symptom deadlock` searched GitHub commit
messages with symptom phrasings that never name a lock, channel, or group
(for example "fix deadlock" and "all goroutines are asleep"), 25 hits per
query. For each commit that changed Go files, it checked out the parent
revision and ran the four checks, with test diagnostics included, on the
packages the fix touched. The binary was built at `04036b9` (SHA-256
`a6c383e25aeb6afc278f90824d74ba8bb2fa9a5f1083c79de5972a8abf44fef4`).

Of 186 hits, 104 were not Go, 31 parent revisions did not build, six had no
usable Go change or parent, and 45 were analyzed. The checks reported nothing
on all 45.

## Labels

Every analyzed commit was reviewed against its diff and parent source. The
[label ledger](deadlock-fixes-2026-09-24.tsv) records one verdict each:

| Label | Count |
| --- | ---: |
| In the class of one of the four checks | 0 |
| A real deadlock or hang of another shape | 35 |
| Not a deadlock fix | 10 |

The 35 real deadlocks are 33 distinct fixes; two commits appear in forks.

| Shape | Count |
| --- | ---: |
| Channel send or receive whose partner is missing or has exited | 12 |
| Blocking I/O, pipe, or network call with no deadline | 11 |
| Self-lock, recursive lock, or missing unlock | 4 |
| Lock held across a callback or a blocking operation | 3 |
| Database lock or connection-pool exhaustion | 2 |
| Missed wakeup, `select{}`, or an unbounded join | 3 |

Three fixes resemble the targeted shapes but fall outside them: in
`hannesrauhe/freeps` and `DataDog/ebpf-manager` the lock is taken through an
interface callback, and in `skupperproject/skupper` the lock is held across a
send whose receiver can also exit on stop.

The four checks prove a deadlock on every path through fresh local resources.
That shape fails on its first run and rarely ships. The fixed deadlocks are
conditional: a partner that is missing on an error, shutdown, or configuration
path.

## Missing-partner follow-up

The [partner ledger](deadlock-fixes-2026-09-24-partners.tsv) relabels the 11
distinct missing-partner fixes by channel origin, partner visibility, trigger,
loops, and select escapes. Only four use a channel created in the waiting
function with every partner visible, and each of those depends on something
else: a zero-iteration loop, a capacity computed as zero, workers exiting
inside a loop, or a partner that is stuck rather than gone. A missing-partner
check limited to fresh local channels would catch none of them.

The recurring shape is a long-lived service loop. In `gocronx-team/cron` and
`ErnestK/MCPSprut`, methods send on a struct-owned channel whose only receiver
is a run loop that returns on shutdown, so a send after shutdown blocks
forever. `fiatjaf/nak` is the receive-side form: a struct-owned channel closed
only on the success path. This motivates a service-loop lifecycle check rather
than more cycle proofs.

The sample is small, found by commit-message search, and skewed toward smaller
projects.
