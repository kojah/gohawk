# Exact acquisition errors matched against context sentinels

This cohort preserves two production-source labels from batch 62 in
`ozontech/cute`, pinned to `9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3`.
The original audit verdicts remain in
[batch-62-findings.tsv](../audits/batch-62-findings.tsv).

The false positive is `doRequest` in `roundtripper.go`: an early return
selected by `errors.Is(httpErr, context.DeadlineExceeded)` cannot leave a
successful acquisition's response unowned. Commit `f958f91` recognizes that
guard only for the exact acquisition error and the known nonnil context
sentinel. Joined errors, unrelated errors, and possibly nil targets do not
establish this implication; minimized fixtures cover those distinctions.

The true-positive control in `test.go` remains reported. Despite its name,
that file is production source, not an excluded `_test.go` file.

Run `make precision-regression ROUND=round-59 REQUIRE_SCANNABLE=1`.
The corrected scoped replay checked both labels: the false positive was
absent and the true positive remained. Candidate code was not executed.
