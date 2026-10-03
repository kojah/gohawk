# Lock slot mutation query consolidation

Beads: `gohawk-dho.23.21.14.2`; finite parent inventory `.14`.
Parent source: `8ce6519c`.

`possibleFreshBoundMutex` and `freshOwnerResult` now use shared
`InstructionsWithin` instead of reproducing instruction visitation and its
budget charge. Order and per-instruction cost remain. Completed constructor
census still requires callee-owned allocation results; interrupted discovery
cannot establish freshness.

`boundSlotMutation` and `visibleMutexSlotReplacement` now yield lazy
`CallBindingsWithin` metadata under their live slot-query allowance. Cutoff
vetoes freshness rather than proving replacement absent; the veto is not proof
of a shared write. Exact supplied-owner matching, field identity and visible
replacement policy remain local. The path-composition binder has no live query
allowance and keeps its existing default mapping rather than acquiring a new
cap. No private/publication/release guarantee is added.

Actual SSA tests distinguish an observer from a visible shared-slot replacement,
check zero-child cutoff with a live parent, and recover completed answers with
a fresh child. Constructor controls distinguish fresh, borrowed and mixed
returns. The restored parent overlay compiles and fails the cutoff veto
assertions for both observer and replacement cases, without a build error or
panic. Receipt: `.build/goal-lock-slot-parent/test.log`.
Full lock tests pass (17.371s), including constructor/escaped-field accepted and
diagnostic fixtures and publication boundaries. Receipts:
`.build/goal-lock-slot-unit.log` and `.build/goal-lock-slot-focused.log`.
Canonical validation: `.build/goal-lock-slot-verify.log`.
Final architecture check: `.build/goal-lock-slot-architecture.log`.

Six read-only all-check scans exit zero with empty stderr. Complete parent/current
payloads agree: lock fixtures (116), private-read fixtures (6), pinned Skywalking
(0). Pins, scopes, exits and hashes: `.build/goal-lock-slot-scans/scans.json`;
complete comparisons: `.build/goal-lock-slot-scans/comparison.json`.
Two traced lock scans preserve all 3,847 complete trace records as multisets,
including 232 final decisions. Traced diagnostics match untraced payloads.
Receipt: `.build/goal-lock-slot-traces/comparison.json`. Event order is not claimed.

Reviewed SHA-256:
`d0876a7d49f8c6450250327c07386d9ede3e9a39e53df9bc34f2c6b3cd11084f`.
It matches the canonical binary. The normalized scanner covers 340 production
files and 2,212 declarations; its five full-body and 33 partial candidate
signatures remain unchanged. This supports a bounded source inventory rather
than semantic duplication absence for the full repository.

Graph tools remain unavailable. Exact source fallback reconciles the finite
release-query, path-binding and declaration-class inventory in the maintained
completion audit. Closure after validation/publication does not close broader
`.23.21` or the root goal: deferred-loop/producer/capture reconciliation and five
reviewed production FP sites remain open. No full precision replay, local race
run, whole-candidate time-bound guarantee or production FP correction credit.
