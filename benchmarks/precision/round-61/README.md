# Uncertain unlock identity through an opaque owner

This cohort preserves six production-source labels from batch 63. Repository
pins are in `repositories.tsv`; original verdicts remain in
[batch-63-findings.tsv](../audits/batch-63-findings.tsv).

Centrifuge's `startReconnectingIf` acquires `c.mu` and releases it at
`client.go:1758`. Nested callbacks make the captured receiver cell opaque to
the bounded storage query, so the old flow assigned that final unlock a
different identity and reported the return at `client.go:1762`.

Commit `79634eb` makes the heap model retain possible aliases between matching projections of
an opaque owner read and its earlier occupant. The lock classifier consumes
such an unlock as uncertainty, not an exact release guarantee. Sibling fields
and different constant-index elements remain distinct. Exact identity is not
inferred from an opaque read.

Three older Centrifuge labels in `subscription.go` were already absent before
this correction and are controls, not newly fixed sites. XD supplies two
genuine missing-unlock controls: error returns from `Save` still leave its
mutex held.

Run `make precision-regression ROUND=round-61 REQUIRE_SCANNABLE=1`.
The corrected replay checked all six labels without exclusions: all four
false positives were absent and both true positives remained. Candidate code
was not executed. The traced Centrifuge invocation also preserved valid JSON
output and recorded the final unlock's identity uncertainty once.
