# Producer launch order

Bead: `gohawk-dho.23.13`. Parent: `247f5d0`.

The producer count previously treated all distinct workers as competing sends,
including workers launched on mutually exclusive branches. Actual SSA for
`helpers.alternativesBeforeCommonWorker` shows two alternative launch blocks
joining before a third launch and two receives
(`.build/goal-producer-launches-ssa.log`). At most two workers start, but the
parent reports three excess sends. A simpler alternative-launch control
produces two more false diagnostics
(`.build/goal-producer-launches-parent.log`).

The existing count proof now requires contributing launches to form a
single dominance chain. The most recent launch is the chain frontier: a new
launch either follows it by dominance, precedes it by dominance, or makes the
count unknown. Dominators of the frontier are themselves ordered, so this
checks the complete set without an additional pairwise scan. Shared instruction
dominance orders same-block launches too. A per-candidate reachability test
against the reported worker would not suffice for the reconverged third worker.

Unordered launches return the existing `producer-count-unknown` proof at the
count boundary. This conservatively misses real excess sends in some
independent conditional-worker patterns. Channel identity, loop uncertainty,
ordered sends and folded source attribution keep their existing policies.
The change extends the existing count evidence family rather than introducing
a second reporting proof. Its function remains cohesive around the producer
count and receiver comparison despite crossing the 60-line review trigger.

Four fixture controls in `helpers/launch_choices.go` cover alternative launches,
alternatives before a common worker, serial workers and nested ordered workers.
A shared trace-test helper checks outcomes at every send source for this
boundary and the preceding folded-source boundary. Final focused tests pass
(`.build/goal-producer-launches-final-controls.log`). A parent-source overlay
fails five accepted diagnostic assertions and five unknown trace assertions,
with terminal exit 1 (`.build/goal-producer-launches-parent-overlay.log`).

Complete all-check scans of `producerlifecycle` and `helpers` both exit 0 with
empty stderr under fixture GOPATH, modules disabled, GOWORK off and CGO disabled.
The merged payload changes from 24 to 19 findings: exactly five producer alerts
in `launch_choices.go` disappear, no diagnostics are added, and all other
payloads remain unchanged. Receipts and full differences are in
`.build/goal-producer-launches/comparison.json`.

Frozen parent `.build/goal-producer-alternates-reviewed` SHA256:
`584a039033b2ffa308a5a2b2cea57cba190f74c426fba1f17765d12774a21f10`.
Frozen current `.build/goal-producer-launches-reviewed` SHA256:
`242d2010dd413921ced81b9d70040a74f5f8be1c8b86d9c198801017f3925ded`.

Canonical `make verify VERIFY_TIMINGS=1` passed all eight targets with
terminal exit 0; the ordinary suite took 51 seconds and local dogfood 22
seconds (`.build/goal-producer-launches-verify.log`). The final architecture
check also passed (`.build/goal-producer-launches-final-architecture.log`). No full precision replay or local race run was performed.
No historical production FP credit is claimed: seven recorded production sites
plus Rune publication remain open. This review does not establish completion
of every producer protocol boundary or the broader consolidation objective.
Graph tools were unavailable; evidence is exact source, actual SSA, assertions
and scoped executable comparisons.
