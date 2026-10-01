# Indexed stores in returned slice owners

This five-label cohort preserves batch 63's three sandbox file-cleanup false
positives, plus the production resource-leak controls from rounds 59 and 60.
Exact repository pins are in `repositories.tsv`. Original source judgments
remain in the frozen audit ledgers.

Sandbox's `prepareFiles` opens up to three files into a made slice. Success
returns that slice; failure passes it to `closeFiles`, which closes each
non-nil element. The original trace already classified the cleanup helper as
unknown. The false reports came from success: the shared returned-owner query
followed the slice to its backing array without checking stores selecting
elements through the slice itself.

Commit `224f3be` retains that backing-owner query and then checks the slice's
own referrers with the existing storage/containment traversal. No separate
indexed-collection flow engine is added. Containment remains possible ownership,
not an exact release guarantee. Fixtures retain reports for another returned
collection and for a return that drops the populated slice. The new shared
tests fail against the old implementation for both fixed and dynamic element
stores and pass with the correction.

Run `make precision-regression ROUND=round-63 REQUIRE_SCANNABLE=1`.
The replay checked all five labels without exclusions: all three false
positives are absent and both true-positive controls remain reported. Only
static analysis ran; candidate tests, generators and applications were not
executed. Canonical `make verify` and focused lifecycle/resource tests pass.
This is a targeted three-repository cohort, not a new audit batch or a full
cumulative precision-regression run.
