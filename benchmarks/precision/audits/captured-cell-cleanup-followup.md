# Later deferred cleanup through a reassigned cell

Commit `b54adbe` corrects the speedtest response-body report from batch 63.
The [three-site ledger](captured-cell-cleanup-followup.tsv) records exact pins,
source locations and package-scoped replay results. Original batch verdicts
remain unchanged.

The fallback request closes the first body before replacing the response.
A deferred literal closes whichever response remains. The exact completion
query cannot establish identity across the cell's multiple writes; treating
the literal as transparent then falsely reports the first acquisition.

The classifier now reuses `capturedCellCleanup` for later deferred literals,
as it already did for registrations before acquisition. Positive cleanup
through the captured cell makes the obligation unknown. This is not an exact
release guarantee and can miss leaks caused by overwriting an unclosed value.
Fixtures retain diagnostics for a different captured response, a read-only
literal, and a conditional registration leaving another path uncovered.

Scoped static scans used the corrected binary with `-enable-all -json`:

- speedtest: from `third_party/speedtest-go`, analyze `./speedtest`;
- Cute: analyze `.`;
- Basecamp: analyze `./internal/commands`.

The false positive is absent and both production-source true-positive controls
remain reported. The speedtest trace labels the defer `captured-cell-may-cleanup`
with outcome `unknown`, and JSON output remains valid. The current CLI excludes
test files; the three historical speedtest test-file labels were not replayed.
The root proxy module has known load errors, so this is package-scoped evidence,
not a clean whole-repository replay. Candidate code was not executed.

Validation: focused resource tests and canonical `make verify` pass. No full
precision audit or cumulative precision-regression run was performed.
