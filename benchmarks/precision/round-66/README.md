# Conditional lock acquisition through a Boolean field getter

This cohort preserves batch 63's production false positive in pion/sctp at
`a09fb03516289d7cd89bc589ac49ee84ac331c62`, `stream.go:350:2`, alongside two
genuine missing-release controls in majestrate/XD at
`b905a14ecfeceaa21a5dde82b52f164d075002ab`.

[WriteSCTP](https://github.com/pion/sctp/blob/a09fb03516289d7cd89bc589ac49ee84ac331c62/stream.go#L311-L350)
locks and unlocks under two calls to
[isBlockWrite](https://github.com/pion/sctp/blob/a09fb03516289d7cd89bc589ac49ee84ac331c62/association.go#L1279-L1281).
The getter only reads a Boolean field initialized at construction. The actual
SSA has separate getter call results; treating them as independent branch
outcomes invents an unreleased return.

The fix reuses the existing loaded-acquisition uncertainty boundary. An exact
getter that selects one parameter field, loads it, and returns it is treated
like a direct Boolean load. This is not a proof of immutability or cleanup.
Changed-field leaks remain a deliberate coverage gap. Constant getters,
computed predicates and unconditional acquisitions keep their prior behavior.
Local fixtures pin those distinctions, and tracing reports the existing
`loaded-acquisition-guard-unknown` decision rather than a proved release.

An initial seven-label comparison also attempted the four original SCTP
goroutine controls at `association_test.go:5197:2`, `5209:2`, `5289:2`, and
`5301:2`. All four are absent with both the parent binary containing `1075818`
and the changed binary. These historical test findings are outside the current
production-only profile following removal of test analysis, not losses caused
by this lock change or corrected review judgments. Their original batch 63
labels remain frozen; they are not included as passing controls in this cohort.
The parent still reports the lock FP and retains both XD lock controls.

Run `make precision-regression ROUND=round-66 REQUIRE_SCANNABLE=1`.
Candidate tests, generators and applications are not executed. This is a
two-repository scoped replay, not a fresh audit or a full cumulative precision
run. Focused lockorder tests and canonical `make verify` pass.
The final three-label replay removes the one FP and retains both TPs, with no
unscannable exclusions. The corrected production trace confirms the unknown
loaded-guard decision at `stream.go:325:19`.
