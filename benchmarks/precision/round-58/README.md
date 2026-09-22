# Follow-up of the 337 unresolved audit sites

This cohort preserves the 85 absent sites from the
[337-site follow-up](../audits/followup-337.md), plus ten known-bug controls.
Three of the absent sites were already absent at the starting revision and
are not credited as new fixes. Original source judgments and all remaining
reports are recorded in the follow-up ledger, not silently dropped here.

The controls include direct response decoding, a file passed to a scanner,
explicit excess sends, an unreaped process, a worker abandoned on error, and
a genuine missing unlock. Five other genuine pfSense helper leaks are accepted
false negatives of the paired-error uncertainty rule; they remain documented
as bugs in the audit and are deliberately not passing true-positive labels.

Run with `make precision-regression ROUND=round-58`. The canonical runner
enables all checks and test-source analysis, without executing candidate code.
Per-package follow-up receipts establish all 85 absences independently of
whole-repository loading. Whole-repository exclusions must not be counted as
passing labels. This initial cohort has no whole-repository findings census;
it tests the reviewed labels, not every diagnostic emitted by these projects.

The first 96-label replay exposed a geesefs logger warning that remained in
the canonical all-checks profile despite an earlier isolated absence. That
site stays unresolved in the full audit ledger; it was removed from this
passing-label selection, not relabelled or counted as a verified fix.

Final replay: 70 labels checked, with all 60 FP absences and ten TP controls
holding. Twenty-five FP labels in six incompletely loadable repositories are
excluded and have no whole-repository confirmation stamp. Their scoped
evidence remains in the full follow-up ledger. This is a passing gate with
explicit exclusions, not a 95-label clean whole-repository scan.
