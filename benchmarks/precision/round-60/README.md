# Deferred release through a captured owner

This cohort preserves three production-source labels from batch 63 in
`basecamp/basecamp-cli`, pinned to `d91fc7b3ae5ee3c54a7fea389f59e791173e647b`.
Original verdicts remain in
[batch-63-findings.tsv](../audits/batch-63-findings.tsv).

The queue drain acquires `q.edges`, registers a deferred literal that unlocks
it, and temporarily releases and reacquires it around callbacks in a loop.
Commit `3850cf8` makes the shared lifecycle mapper prove the captured owner cell stable and
matches the embedded mutex's exact path. It accepts this deferred cleanup
without treating a sibling field or a reassigned owner as the same mutex.

The response-body false positive was already absent before this change and
is retained as a control, not credited as a new correction. The MCP worker's
true-positive resource label remains reported.

Run `make precision-regression ROUND=round-60 REQUIRE_SCANNABLE=1`.
The scoped replay checked all three labels: both false positives were absent
and the true positive remained. Candidate code was not executed.

A separate four-label replay of `centrifugal/centrifuge-go` found three older
false positives already absent, but `client.go:1762:2` remains reported.
That unresolved site is not included in this passing cohort and its original
false-positive verdict remains unchanged.

The remaining Centrifuge site was subsequently corrected and verified with
two genuine missing-unlock controls in [round 61](../round-61/README.md).
