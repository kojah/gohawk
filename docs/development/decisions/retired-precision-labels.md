# Retired precision labels

The experimental `processownership/detached` check is retired. Fire-and-forget
launching alone cannot distinguish a process leak from an intentional browser,
daemon, or relaunch lifecycle; requiring a parent to wait may contradict that
lifecycle. Fifteen executable true-positive labels from rounds 2, 3, 5, 8, 12,
16, 18, 19, and 21 were removed. Their historical audit verdicts and scan
baselines remain unchanged, not claims of current coverage. The focused
`processownership/missing-wait` check remains supported with its existing
boundary; discarded handles are not relabeled as missing-wait defects.

The `exitpolicy` analyzer is retired in full. Skipping a defer on process exit
does not establish that its cleanup matters after termination; the audit was
dominated by intentional fatal paths and examples. Rather than accumulating
intent-based exceptions, its implementation and two round-22 executable
labels were removed. Historical findings remain as records of the old scans,
not current coverage; in-flight batch-49 exit findings are retired checks.

The unconditional channel timer/ticker cleanup contract has been removed.
Twenty-two timer-only labels (20 previously marked true positive, two false
positive) were removed from the executable cohorts after checking their
pinned acquisition sites. Missing `Stop` alone does not establish a leak with
Go 1.23+ GC semantics. This is a narrower claim, not proof that retained
workers in those historical examples are correct. Historical findings and
audit narratives are preserved; timer descriptions below describe the old
scans, not current coverage.

The experimental `goroutineownership/detached` check is retired. Seven
true-positive policy labels from rounds 6–9 and 11 were removed from the
executable cohorts: absence of a recognizable owner is not proof of a bug.
Historical scan and audit records remain unchanged. The focused
`goroutineownership/unjoined` and `producerlifecycle` checks remain supported.

The unjoined check now declines early `WaitGroup.Done` alone: it may announce
readiness instead of completion. The round-10 moov-io/rtp20022 early-Done
true-positive replay label was removed as an accepted coverage gap, not changed
to a false positive. Independent terminal or deferred completion obligations
remain checked; the historical finding and audit review are preserved.

The experimental `lockorder/mismatched-release` check is retired. Releasing a
lock with the method that does not match its acquisition is fatal on the first
run of that path, so the bug rarely ships: the check found one true positive
in about 1,000 audited repositories. Its one executable label, the round-58
refraction-networking/utls false positive at `common.go:1104:2`, was removed;
the round-58 count drops from 95 to 94. The mode tracking it relied on remains
for `lockorder/read-lock-write`, and the lock walk still ignores deferred
acquisitions, the fix that label guarded.

Labels named analyzers the project has since withdrawn:
channelownership (10), errorownership (7), determinism (7),
closedomain (6), apishape (5), contextpolicy (5),
and wirepolicy (3).

They are removed rather than left in place. A true-positive label for a
withdrawn analyzer can never be present again, so it fails every replay for a
reason no change can fix; a false-positive label for one trivially remains
absent, which is a pass nobody earned. The verdicts were real reviews, and the
audit records that describe them are kept; what is gone is the executable
claim, because the code that made it no longer ships.
