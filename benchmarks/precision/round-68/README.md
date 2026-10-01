# Logger options appended to a foreign owner

This cohort preserves batch 62's production false positive in twmb/kcl at
`5290cb05bcc421a239e327ba11408bc4e27bd2dd`, `client/client.go:1445:13`,
alongside the production resource-leak controls in Cute and Basecamp pinned
in rounds 59 and 60.

[parseLogLevel](https://github.com/twmb/kcl/blob/5290cb05bcc421a239e327ba11408bc4e27bd2dd/client/client.go#L1422-L1456)
opens a file, wraps it with `kgo.BasicLogger`, captures that logger in a
`kgo.WithLogger` option, then appends the option to the client's stored slice.
The SSA has a merged file value and a compiler-built variadic array holding
the option. The old wrapper-publication query missed the appended values;
the parent binary still reports the acquisition.

The existing bounded wrapper-chain proof now reuses `ssaflow.AppendedValues`
when an explicit append result is published to foreign storage. Each wrapper
step retains its existing effect boundary; spread slices remain outside this
query. This supplies uncertain ownership, not proof of a close or a promise
about logging or process lifetime. Wrapper-publication functions moved from
the classifier into the existing ownership evidence file.

The minimized `appendedConfiguredWrapper` fixture fails against the parent
code and passes with the fix. An unrelated wrapper and a read-only observer
remain diagnostic. A discarded local slice of wrappers remains a documented
false-negative gap because append can become opaque consumption before the
publication query. The trace pins `wrapper-stored-on-foreign-owner` with outcome
`unknown`; the production trace records that label on the stored option slice.

All three production labels pass without unscannable exclusions. KCL's 26
historical test-file TP labels are outside the current production-only profile
following the deliberate removal of test analysis; they are not lost controls
or passing coverage for this correction. Their original reviews remain frozen.

Run `make precision-regression ROUND=round-68 REQUIRE_SCANNABLE=1`.
Candidate tests, generators and applications are not executed. This is a scoped
correction, not a fresh repository audit or a full cumulative precision run.

Focused resource tests, tracing assertions and canonical `make verify` pass.
Comparing all findings on the three pinned repositories against the parent
binary shows only removal of the reviewed KCL FP, with no new reports or other
removed findings. The final architecture checks pass too.
