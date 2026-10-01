# Read-lock handoff on success and failure

This cohort preserves batch 63's production false positive in fiorix/go-diameter
at `c7794c55a5412a3d91b17165971be4c6bc6b3ced`,
`examples/s6a_proxy/service/util.go:82:2`, and two genuine lock-leak controls in
majestrate/XD at `b905a14ecfeceaa21a5dde82b52f164d075002ab`.

[acquireConnection](https://github.com/fiorix/go-diameter/blob/c7794c55a5412a3d91b17165971be4c6bc6b3ced/examples/s6a_proxy/service/util.go#L51-L87)
returns with the read lock held on both success and failure. Its documented
contract requires the caller to release the lock. The slow path temporarily
releases it, acquires and releases the write lock, then reacquires the read lock.
The merged error result is not a literal nil, so the existing successful-return
proof missed this handoff. The parent binary reports the site.

The fix extends the existing held-for-caller proof with definite retention on
every normal return. It also corrects return-state aggregation: an SSA return
reached both holding and without holding the lock supplies possible retention,
but no definite retention. Both observations come from the same lock-flow
walk. Successful-return and Boolean caller-release contracts consume that
evidence too, rather than treating a held-path witness as an all-path guarantee.
This establishes a handoff, not eventual cleanup by the caller.

`lockorder/return_handoff.go` pairs the accepted read-lock handoff with merged
held/released returns and an opaque-error leak. Trace coverage requires
`held-for-caller-proven`; the pinned production trace emits that accepted
decision at `util.go:56:12`.

An eight-label scoped replay also checked Centrifuge's four corrections and
SCTP's conditional-lock correction. All six FPs are absent and both XD TPs
remain reported, without unscannable exclusions. Candidate tests, generators
and applications were not executed. Original batch verdicts remain frozen.

Run `make precision-regression ROUND=round-67 REQUIRE_SCANNABLE=1` for the
three-label permanent cohort. This is a scoped correction, not a new audit or
a full cumulative precision-regression run.

The permanent cohort passes all three labels without exclusions. Focused
lockorder tests, tracing assertions and canonical `make verify` pass. Comparing
all findings on the four scoped repositories against the parent binary shows
only removal of the go-diameter FP: no new reports or other removed findings.
