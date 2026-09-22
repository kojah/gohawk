# Process and cancellation follow-up checkpoint

Four of the nine findings in the frozen 207-site input are now absent; five
remain active. Both earlier sonar corrections and all seven reviewed bug
controls remain intact. All 13 pinned package scopes loaded successfully.
The [18-site ledger](followup207-process-cancel.tsv) preserves original verdicts.

The corrections are tuple-returned command-factory uncertainty (Mutagen),
receipt of the exact child context's Done channel (Gleam), constant helper
error-result feasibility (Sonar), and an exact successful-Start command merge
(gonc). The remaining sites concern a detached process boundary, three
process-lifetime signal registrations, and an expired timer; none is silently
waived because of its application, function name, or test intent.

The command merge is admitted only in the immediate, non-cyclic success block.
Its phi receives the exact started command from that edge. The ordinary flow
then retains that precise non-nil assumption; a later nil replacement does not
inherit it. Direct and unrelated-command controls remain checked. Deferred
Process-field guards whose distinct loads prevent an exact proof are explicitly
unknown, not claimed to reap the process. An additional Boolean guard or a
visible field replacement in the deferred waiter retains the diagnostic.

Validation includes diagnostic/accepted fixtures, trace assertions, focused
tests, lint, and focused race tests. The canonical static-only candidate is
`.build/gohawk-followup207-process-merge-v3`, SHA-256
`47943b8e0437e06214ffbd42f3ae6465d140dfd24bb5d524e8307db37d55e7b4`.
Receipts are under `.build/followup207-process-merge-v3/`; commands use
`-enable-all -gohawk-include-tests -json`, the original revisions, and the same
environment recorded in every receipt. Candidate code was not executed.
No process/cancellation findings were added relative to the preceding
`followup207-flow-v1` replay; the gonc finding was removed.

Earlier `termination-v1` and `termination-v2` replays are not final evidence:
the former captured an in-flight select decoder defect, and the latter was
stopped when an in-flight lock-state extension caused excessive resource use.
Those failures were repaired and the complete scopes rerun. The combined
goroutine replay subsequently retained all 56 bug controls, and `make verify`
passes. The dependent implementation checkpoint is `511ed2b`.
