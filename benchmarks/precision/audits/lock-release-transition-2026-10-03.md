# Exact lock completion transition consolidation

Beads: `gohawk-dho.23.21.14.1`. Parent: `56af059e`.

Called and spawned completion repeated held-identity/value traversal, exact
query admission, cutoff checks and released/held/guard updates. One
`transferCompletedUnlocks` now selects a typed Called/Started completion reason
from the instruction and owns those mechanics. Only synchronous calls ask
possible-release coverage, record its witness and weaken the definite-held
claim. Conditional workers retain their obligation. Deferred registration and
complete summarized release/reacquire sequences remain separate policies.
Rationale links move with the owning exact and possible decisions.

Compiled SSA controls pass against the parent and consolidated implementations
for exact calls/workers, conditional calls/workers, opaque callbacks and defers.
They check held/released/guard state and synchronous uncertainty. Full lock
package tests pass (16.054s), including query-cutoff/fresh recovery, late-cut
publication and existing accepted/diagnostic fixtures.
Receipts: `.build/goal-lock-transition-parent.log` and
`.build/goal-lock-transition-focused.log`.
Canonical validation: `.build/goal-lock-transition-verify.log`.
Final architecture validation after documentation:
`.build/goal-lock-transition-architecture.log`.

Six read-only parent/current all-check scans exit zero with empty stderr.
Complete payloads agree for lock fixtures (116), private-read fixtures (6) and
pinned Skywalking (0). Skywalking's private-owner corrections remain intact.
Pins, scopes, exits and hashes: `.build/goal-lock-transition-scans/scans.json`;
complete comparisons: `.build/goal-lock-transition-scans/comparison.json`.
Two additional traced lock scans preserve all 3,847 complete trace records as
multisets, including final decisions and possible-release evidence. Traced JSON
diagnostics match the corresponding untraced payloads. Record order is not
claimed. Receipt: `.build/goal-lock-transition-traces/comparison.json`.

Reviewed binary SHA-256:
`09f3b71789c9d1e5180e21d2b4516ad1f0ff963f55cecf89a2840a1ae4755f71`.
It matches the canonical binary. The normalized scanner covers 340 production
files and 2,212 declarations; all five full-body and 33 partial candidate
signatures remain unchanged. This does not negate the source-demonstrated
loop consolidation or prove semantic duplication absent elsewhere.

Graph tools remain unavailable; source fallback reviewed release queries,
call-site bindings and declaration classes. The parent lock review remains
open for slot mutation/path-composition disposition in `.14.2`. Existing
independent query costs are not converted into a whole-candidate time bound.
No full precision replay, local race run or production FP correction credit.
Five reviewed production FP sites remain unresolved.
