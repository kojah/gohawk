# Producer selected callback capture

Bead: `gohawk-dho.23.17`. Parent: `342ac68`.

## Evidence and consolidation

A caller launches two channel sends, receives once, then invokes either of two
closures that receive again. The summary engine cannot bind the callback phi,
so the producer classifier falls back to opaque-use evidence. It previously
checked captures only when the called value was a direct MakeClosure, missing
the selected callbacks. Parent fixtures report the second send incorrectly
(`.build/goal-producer-callback-choices-parent.log`). Actual caller and worker
SSA is in `.build/goal-producer-callback-choices-ssa.log`.

Process ownership already used a bounded reaching-closure capture fold. That
mechanic now belongs to `lifecycle.ProvePossibleClosureCaptureWithin`, with its
controls beside the shared implementation. A positive proof establishes only
possible structural capture; it does not prove invocation, receiving, Wait or
completion. Phi alternatives are transparent; conversions and loads remain
opaque. Cutoff is unknown and shares the supplied allowance. Graph work retains
its independent bounds.

The process adapter maps a positive capture to unknown Wait participation,
preserving its prior decision. The producer classifier uses positive captures
from selected call values and arguments as unknown receiver evidence, never as
an exact receive count. Budget cutoff retains the existing budget reason.
Thus two consumers share the mechanism while keeping their diagnostic policy.

The earlier `gohawk-dho.23.16` complete-summary channel-phi hypothesis was ruled
out by actual SSA: the engine returns `protocol-channel-binding-unknown` before
summary receive counting (`.build/goal-producer-receiver-identity-parent.log`).
Its exploratory test was removed and no policy was changed for that unsupported
case. This does not prove every non-budget identity boundary is complete.

## Controls and scoped comparison

Selected drains are accepted; a selected callback capturing only an unrelated
scalar retains the excess-send diagnostic. Both selected-drain source decisions
carry unknown `receiver-helper-unknown`. Shared capture controls cover a mixed
choice, unrelated captures, an intentionally opaque conversion, child cutoff
without parent exhaustion and fresh complete proofs. Process-choice fixtures
keep the domain adapter's unknown handoff behavior.

Focused lifecycle/producer/process tests and lint pass. A parent producer-file
overlay fails the accepted diagnostic and trace outcome assertions. A shared
capture-polarity overlay also fails the positive capture assertion and producer
fixtures, confirming that positive capture cannot be conflated with query
unavailability. Logs are `.build/goal-producer-callback-choices-*-overlay.log`.

Four complete all-check fixture receipts exit 0 with empty stderr under CGO
disabled, fixture GOPATH, modules disabled and GOWORK off. Producer fixtures
change from 23 to 22 findings: only the false selected-drain alert disappears.
Process fixtures retain all 40 findings, and no diagnostics are added in either
scope. Full merged payload differences are in
`.build/goal-producer-callback-choices/comparison.json`.

Frozen parent `.build/goal-producer-receiver-budget-reviewed` SHA256:
`69ebbe9b29043bf032971ebd64e6b6281b30a13ff2b1769985095244567a6e66`.
Frozen current `.build/goal-producer-callback-choices-reviewed` SHA256:
`88127c0832bf137c25cf9805679677a0fa8f87133904428967af4a98c7714e1f`.

Canonical `make verify VERIFY_TIMINGS=1` passed with terminal exit 0
(`.build/goal-producer-callback-choices-verify.log`). Final architecture
validation passed after the documentation update
(`.build/goal-producer-callback-choices-final-architecture.log`). No full precision replay or
local race run was performed. Graph tools were unavailable; evidence is exact
source, actual SSA, assertions and scoped executable comparisons. Seven
recorded production FP sites plus Rune and broader semantic/partial-duplication
completion remain open; no historical production FP credit is claimed.
