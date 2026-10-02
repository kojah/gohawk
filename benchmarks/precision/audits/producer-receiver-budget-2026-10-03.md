# Producer receiver budget cutoffs

Bead: `gohawk-dho.23.15`. Parent: `94ff2c0`.

Receiver helper counting binds a concurrency summary and then compares its
receive-channel identity under the same allowance. The identity cutoff was
previously interpreted as no matching receive. The real SSA one-receive helper
reproduces a known zero count at allowance 5 on the parent
(`.build/goal-producer-receiver-budget-parent.log`). Losing a receiver that way
can support false excess-send evidence when another receive remains visible.
No historical production diagnostic removal is claimed from this control.

The helper classifier now returns unknown with `receiver-budget-exhausted`
after summary binding, receive identity or incomplete-worker use classification
exhausts the allowance. A partial
positive count is discarded too. A focused summary-count helper keeps the
binding orchestration and operation identity/counting responsibilities clear;
normal complete and asynchronous receiver policies retain their behavior.
This does not resolve all non-budget identity ambiguity or indirect receivers.

`receiver_budget_test.go` builds actual SSA and checks every allowance through
first completion for cold and warm one/two-receive helpers and a non-receiving
worker with an incomplete summary. Every cutoff must
be unknown, carry the budget reason and retain zero partial count. Completed
queries retain the exact expected receive count. Final focused controls pass.
A parent-source overlay fails the cutoff/count assertions
(`.build/goal-producer-receiver-budget-parent-overlay.log`).

Complete all-check fixture scans of `producerlifecycle` and `helpers` both exit
0 with empty stderr, under fixture GOPATH, modules disabled, GOWORK off and CGO
disabled. Full merged payloads are identical at 21 findings; no findings are
added or removed. Receipts are in
`.build/goal-producer-receiver-budget/comparison.json`.

Frozen parent `.build/goal-producer-fallback-reviewed` SHA256:
`54234ae7c4f87dc19148520c9668be3eb75abba4c1f4e48ab9989bc283e87756`.
Frozen current `.build/goal-producer-receiver-budget-reviewed` SHA256:
`69ebbe9b29043bf032971ebd64e6b6281b30a13ff2b1769985095244567a6e66`.

Final canonical `make verify VERIFY_TIMINGS=1` passed all eight targets
with terminal exit 0 (`.build/goal-producer-receiver-budget-final-verify.log`).
The final architecture check also passed
(`.build/goal-producer-receiver-budget-final-architecture.log`). The first canonical lint pass identified the test's integer loop
form; it is corrected to the repository's integer-range convention. The
subsequent nesting lint finding is corrected with a flat cutoff guard, and
final focused tests and lint pass. No full
precision replay or local race run was performed. Graph tools were unavailable;
evidence is exact source, real SSA, allowance controls and scoped executable
comparisons. Seven recorded production sites plus Rune and broader semantic
and partial-duplication completion remain open.
