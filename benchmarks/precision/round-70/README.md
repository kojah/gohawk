# Immediate successful-start process guards

Tencent's pinned
[forkBus](https://github.com/TencentCloud/tencentmeeting-cli/blob/e631b355da2b001d24b82f453b65d96f39c59865/internal/event/spawner/spawner.go#L113-L123)
checks `Start`, then immediately tests `cmd.Process != nil` and releases the
handle. The nil successor and the release successor merge at the final return.
The parent reports the impossible nil path as missing wait ownership.

The existing narrow guard proof now supplies a non-nil value to the shared
flow's fixed-value assumptions. It requires the immediate successful-Start
edge, a mutation-free four-instruction guard, and exact command identity.
This replaces the separate return-local exception rather than creating another
flow walk. Only the guard load is fixed; later fields and opaque calls gain
no assumption. Additional Boolean conditions, field replacement, and a guard
on another command remain diagnostic controls.

The new merged-return fixture fails against parent `e13c006`; corrected
Release and Wait forms pass alongside those controls. The structured trace
records `successful-start-process-non-nil` evidence and a final
`wait-ownership-proven` decision for the pinned daemon. The old
`impossible-nil-process-return` trace reason belonged to the removed return
exception. Trace assertions share one table of expected boundaries.

Parent binary SHA-256:
`2d4f92f8f00cd43cb884aba688fd578bc583f816924a367139de28be2a50195a`.
Corrected binary SHA-256:
`ddc506d2665febea8bfdef02d207180561a60d091b5f4a05d092f30594dc5203`.
It implements production correction `d899cfd` and was built before the commit.
Both complete scans of the two pinned repositories succeeded. The correction
removes the reviewed Tencent FP and retains openfaas's production missing-wait
control at `executor/serializing_fork_runner.go:105:12`. Comparing all emitted
findings shows exactly that one removal, with no additions or other removals.

Run `make precision-regression ROUND=round-70 REQUIRE_SCANNABLE=1`.
Candidate tests, applications, and generators are not executed. This is a
focused correction cohort, not a fresh repository audit or a full historical
precision replay.

Focused process fixtures, structured trace assertions, canonical `make verify`,
and final architecture checks pass.
