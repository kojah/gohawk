# Producer branch attribution

Bead: `gohawk-dho.23.12`. Parent: `b8469c3`.

## Reproduced defect and correction

The concurrency summary's `Operation.Alternates` contains source positions
for the same operation on equivalent branches. `summarizedSends` previously
expanded those positions into separate producer records. The count proof
then counted every position as another execution. A balanced one-send worker
was reported twice, and a two-send worker's first send could also be reported.

Actual SSA for `helpers.equalBranchSend` is captured in
`.build/goal-producer-alternates-ssa.log`: the worker branches to two sends,
then joins at its normal return. The caller receives once. Parent analyzer
assertions fail on both send positions; this is not inferred from Go syntax
alone (`.build/goal-producer-alternates-parent.log`).

Each normalized operation now produces one record with a list of source
positions. The authoritative count proof runs once for that record. Reporting
and tracing attribute the same proof to every source position. Direct sends
have a one-position list. Sequence ordering, channel identity, receiver
classification and loop uncertainty retain their existing rules.

## Controls and comparison

`helpers/branch_sources.go` covers balanced branch sends, balanced pairs,
excess second sends and distinct competing workers. Expected diagnostics keep
both branch positions for true excess sends. The trace regression checks the
accepted or rejected outcome at every source position. Focused package tests
pass (`.build/goal-producer-alternates-final-controls.log`). Restoring both
parent production files through a Go overlay fails six accepted diagnostic
assertions and six trace outcome assertions, with terminal exit 1
(`.build/goal-producer-alternates-counterfactual.log`).

Both binaries scan `producerlifecycle` and `helpers` with all checks enabled,
CGO disabled, modules disabled, GOWORK off, and the fixture tree as GOPATH.
Both `go vet -vettool` runs exit 0 with empty stderr. Merged complete diagnostic
payloads change from 21 to 15 findings: exactly six producer diagnostics in
`branch_sources.go` disappear, no diagnostics are added, and every other
payload is unchanged. Receipts and complete differences are in
`.build/goal-producer-alternates/comparison.json`.

Frozen binaries:

- Parent `.build/goal-aggregate-writes-reviewed`, SHA256
  `0a8048ec0356b4b195935b04f3b713179423f65d3f90c7970929b76dab430c77`.
- Current `.build/goal-producer-alternates-reviewed`, SHA256
  `584a039033b2ffa308a5a2b2cea57cba190f74c426fba1f17765d12774a21f10`.

Canonical `make verify VERIFY_TIMINGS=1` passed all eight targets; the ordinary
suite took 50 seconds and local dogfood 22 seconds
(`.build/goal-producer-alternates-verify.log`). The added trace regression also
passes in the final focused package run. Final formatter/lint and architecture
checks passed with terminal exit 0 after the documentation update
(`.build/goal-producer-alternates-final-fmt.log`,
`.build/goal-producer-alternates-final-lint.log`,
`.build/goal-producer-alternates-final-architecture.log`).

No full precision replay or local race run was performed. This is a bounded
source-attribution/count correction; the seven recorded production FP sites
plus Rune publication remain open. No historical production FP removal is
credited. Graph tools were unavailable; evidence comes from exact source,
actual SSA, analyzer assertions and scoped executable comparisons.
