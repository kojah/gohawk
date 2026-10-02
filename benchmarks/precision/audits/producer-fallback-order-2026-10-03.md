# Producer fallback send order

Bead: `gohawk-dho.23.14`. Parent: `f00fd03`.

## Evidence and correction

An incomplete worker summary falls back to visible direct sends. Those sends
were counted when they could reach the reported send, even if they were on
mutually exclusive branches. One send on either branch followed by one common
send was therefore counted as three against two receives. A separate competing
worker inherited the same inflated total.

Actual SSA is captured in `.build/goal-producer-fallback-ssa.log` and
`.build/goal-producer-fallback-competing-ssa.log`. The minimized fixture has an
opaque callback before the branch, which prevents a complete normalized
protocol. Parent analyzer assertions reproduce three false alerts
(`.build/goal-producer-fallback-parent.log`).

`countProducerSends` now owns the structured count outcome. Contributing launches
retain their dominance-chain requirement, and contributing direct fallback
sends must form a chain within each worker too. One analyzer-local frontier
helper shares the ordering mechanic; identity, instruction order and dominance
come from the existing shared SSA helper. Complete summary operations retain
their normalized sequence order. An unordered set returns the existing
`producer-count-unknown` reason at the count boundary, before receiver comparison.
This conservatively misses some true excess protocols with unordered fallback
sends. No naming, framework, timing or loop-count guess is added.

## Validation

Four controls in `helpers/fallback_choices.go` cover a balanced branching
worker, balanced competition, straight-line excess and nested ordered excess.
Per-source trace assertions cover accepted first sends, unknown totals and
rejected exact excess sends. Focused package tests pass
(`.build/goal-producer-fallback-final-controls.log`). A parent production-source
overlay fails three accepted diagnostic assertions and three unknown outcome
and reason assertions, with terminal exit 1
(`.build/goal-producer-fallback-parent-overlay.log`).

Both all-check scans of `producerlifecycle` and `helpers` exit 0 with empty
stderr, using fixture GOPATH, modules disabled, GOWORK off and CGO disabled.
Complete merged payloads change from 24 to 21 findings: exactly three producer
alerts in `fallback_choices.go` disappear, no diagnostics are added, and all
other payloads are unchanged. Receipts and complete differences are in
`.build/goal-producer-fallback/comparison.json`.

Frozen parent `.build/goal-producer-launches-reviewed` SHA256:
`242d2010dd413921ced81b9d70040a74f5f8be1c8b86d9c198801017f3925ded`.
Frozen current `.build/goal-producer-fallback-reviewed` SHA256:
`54234ae7c4f87dc19148520c9668be3eb75abba4c1f4e48ab9989bc283e87756`.

Canonical `make verify VERIFY_TIMINGS=1` passed all eight targets with
terminal exit 0; ordinary tests took 51 seconds and local dogfood 23 seconds
(`.build/goal-producer-fallback-verify.log`). The final architecture check
also passed (`.build/goal-producer-fallback-final-architecture.log`). No full precision replay or local race run was performed. Graph
tools were unavailable; evidence comes from exact source, actual SSA,
assertions and scoped executable comparisons. This bounded producer correction
does not establish overall architectural completion. Seven recorded production
FP sites plus Rune publication remain open; no historical production FP credit
is claimed.
