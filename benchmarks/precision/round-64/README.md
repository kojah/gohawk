# Transaction cleanup through paired context cancellation

This four-label cohort preserves two Odysee transaction false positives and
two production HTTP-body leak controls from batch 62. The original verdicts
remain in [batch-62-findings.tsv](../audits/batch-62-findings.tsv); the repository
pin is in `repositories.tsv`.

Both transaction sites use `BeginTx` with a `WithTimeout` context and defer
its paired cancel before acquisition. On early prepare/execute failures, that
cancel causes database/sql's rollback watcher to finish the transaction.
The old trace had no cleanup label for that defer and reported those returns.

Commit `51bd633` labels direct or deferred cancellation of the exact BeginTx
context as unknown cleanup. It does not prove synchronous rollback or commit.
One structural context/cancel pairing check serves both this contract and the
existing pre-acquisition cancellation rule. It recognizes results zero and one
from the same standard constructor, including the cause and timed variants;
it does not infer cancellation from a deadline or an unrelated cancel.

Fixtures cover both DB and Conn, all six context constructors, registrations
before and after acquisition, direct calls, another/replaced context,
conditional cancellation, and Begin ignoring the context. The pre-acquisition
query fixture also verifies explicitly canceling a timed context. Incorrect
transaction intent can remain hidden by rollback; this is a resource-lifetime
check, not proof that the transaction's changes were committed.

Run `make precision-regression ROUND=round-64 REQUIRE_SCANNABLE=1`.
The replay checks all four labels without exclusions: both false positives
are absent and both true positives remain reported. A separate scoped scan of
`./apps/watchman/olapdb ./apps/forklift ./internal/test` confirms the same
results. Its valid JSON output and trace show both deferred cancels labelled
`transaction-context-canceled` with outcome `unknown`.

Focused resource tests, canonical `make verify`, and the final public-doc
architecture checks pass. Candidate tests, generators and applications were
not executed. This is a one-repository regression cohort, not a fresh audit
batch or a full cumulative precision-regression run.
