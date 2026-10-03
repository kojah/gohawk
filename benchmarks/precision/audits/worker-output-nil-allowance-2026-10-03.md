# Worker output nil-fold allowance

Beads: `gohawk-dho.44.15`. Parent source: `587ba336`.

The bounded worker-output scan still used a default nil fold. It now passes
its caller-lifetime allowance to `DefinitelyNilWithin` and checks availability
before using a negative result as possible publication. Fresh nil output
arguments remain disabled, and live send-capable channels retain possible
publication. The final caller-lifetime proof consumes cutoff as unknown.
No budget constant, published fact, ownership guarantee or diagnostic policy
is added.

The real SSA parent probe sees `publish(nil:chan int)` and `publish(done)`.
It measures minimum completed limits 3 and 2 respectively. The new regression
cuts at those limits while leaving its outer pool live and recovers original
publication outcomes with fresh queries. Restoring the parent source compiles
and fails both assertions. The first implementation only passed the allowance;
the live-channel test exposed its positive Boolean after the nil fold cut.
The final implementation checks availability immediately after that fold.
This failed iteration is not a passing receipt.

Focused caller/receiver/receive/relay/discovery controls pass alongside the new
regression: `.build/goal-publication-nil-focused.log`.
Actual parent SSA and measured minima:
`.build/goal-publication-nil-counterfactual/probe.log`.
Compiled counterfactual: `.build/goal-publication-nil-counterfactual/test.log`.

Final `make verify VERIFY_TIMINGS=1` passes all eight targets, including ordinary
tests, self-dogfood, static checks and generation.
Receipt: `.build/goal-publication-nil-verify.log`.
Six read-only all-check parent/current scans exit zero with empty stderr and
identical complete payloads: goroutine fixtures128, pinned Openase hook0 and
pinned stargz store1. Pins, scopes and exits are in
`.build/goal-publication-nil-scans/scans.json`; full comparisons are recorded in
`comparison.json`. Openase cutoff silence receives no FP correction credit.
Reviewed SHA-256:
`13cd9864c9be40506e7e107626c3cf9fd759ce6f82bd0b3d1f60bbfc65c7eb07`.
It matches the final canonical binary; these are precommit artifacts.

The normalized scan retains338 production files,2,210 declarations, five
full-body and33 partial groups. All path/name/token candidate signatures match
the parent. Graph tools remain unavailable; exact source fallback was used.
The maintained completion ledger maps the finite goroutine review's original
requirements to current input, classifier, flow, suppression and cost owners.
Distinct guarded/count/historical containment contracts are recorded rather
than fused into one broad identity or cleanup rule.

Final architecture validation after these maintained documentation updates is
recorded in `.build/goal-publication-nil-architecture.log`.
No full precision replay, local race run or production FP correction credit.
Five reviewed production FP sites and the broader consolidation audit remain open.
