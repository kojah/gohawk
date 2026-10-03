# Completion discovery nil-fold allowance

Beads: `gohawk-dho.44.11.9`. Parent source: `8eddaad`.

Three completion-discovery calls still used the default nil fold despite a
live constructor allowance. Channel signals, direct group completion and
deferred group completion now call `DefinitelyNilWithin` with that allowance.
Optional nil handles remain excluded; exact binding and every-return completion
requirements remain unchanged. The existing candidate availability check prevents
a discovered prefix from becoming a diagnostic at cutoff. No new budget,
summary payload, proof family or diagnostic policy is introduced.

`TestCompletionDiscoveryNilAllowance` builds actual SSA for nil channel,
direct nil group, deferred nil group and live channel launches. Child limits
11, 10, 14 and 11 respectively were measured against the parent implementation.
Each now exhausts discovery while leaving the outer candidate pool live and
returns unknown before reporting. Sweeps recover the original handle counts
with fresh completed queries. The parent overlay compiles and fails all four
assertions; it has no build failure or panic. Existing oversized-worker,
partial-census and adapter trace regressions also pass.

Focused receipt: `.build/goal-discovery-nil-focused.log`.
Counterfactual: `.build/goal-discovery-nil-mutant/{test.log,result.json}`.
Six read-only all-check parent/current scans exit zero with empty stderr.
Complete diagnostic payloads agree: goroutine fixtures 128, pinned Openase
hook 0 and pinned stargz store 1. Pins, scopes, binary hashes and exits are in
`.build/goal-discovery-nil-scans/scans.json`; full comparisons are in
`comparison.json`. Openase silence still receives no FP-correction credit.

Reviewed binary SHA-256:
`07a3c8ae525f9bd3dffabdaefa36bcd62668161dc30c8fad6e95bb534c188073`.
It matches the canonical binary. These are precommit build artifacts.
The normalized scanner retains 336 production files, 2,208 declarations,
five full-body and 33 partial groups; every path/name/token candidate signature
matches the parent. Those finite dispositions are not a global duplication
absence claim. Graph tools remain unavailable; exact source fallback was used.

No full precision replay or local race run. Five reviewed production FP sites
and the broader architecture completion audit remain open.

Final `make verify VERIFY_TIMINGS=1` passes all eight canonical targets,
including ordinary tests, generation, vet, formatting, lint, deadcode and
self-dogfood. Receipt: `.build/goal-discovery-nil-verify.log`.
Final architecture validation after the maintained documentation updates is
recorded in `.build/goal-discovery-nil-architecture.log`.
The first post-documentation architecture run rejected two bare analyzer test
references as architecture test names. The maintained ledger now cites their
owning test file; the final rerun is the authoritative receipt.
