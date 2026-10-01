# Returned process-handle owners

This two-label cohort preserves batch 63's sandbox false positive and an
of-watchdog true-positive control. Pins are in `repositories.tsv`; original
verdicts remain in [batch-63-findings.tsv](../audits/batch-63-findings.tsv).

The sandbox starts an `exec.Cmd`, stores `cmd.Process` in a new container,
and returns that container. Its `Destroy` method kills and waits on the handle.
The old proof recognized a returned command or a directly returned process
handle, but missed an aggregate retaining only the process handle.

Commit `24b0f36` makes the process-handle classifier use the existing returned-value containment
query for loads derived from the started command. An aggregate retaining that
handle makes ownership unknown, not a guaranteed wait. Direct handle returns
retain their existing transfer rule. PID-only results, another process's
handle, and an uncovered return do not qualify; discarded local owners remain
diagnostic. Possible containment can miss real leaks and does not prove an
owner eventually performs cleanup.

The of-watchdog control returns on a pipe failure without waiting for the
already-started child. It remains reported.

Run `make precision-regression ROUND=round-62 REQUIRE_SCANNABLE=1`.
The scoped cohort replay checked both labels without exclusions: the false
positive is absent and the true positive remains. No candidate tests,
generators or applications were executed. This is a two-repository regression
cohort, not a new precision audit batch or a cumulative replay.

Focused processownership tests and canonical `make verify` pass. The corrected
sandbox trace records `ambiguous-wait-ownership` with outcome `unknown`.
