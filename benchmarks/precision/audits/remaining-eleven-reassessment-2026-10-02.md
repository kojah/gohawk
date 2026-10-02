# Remaining eleven production findings — 2026-10-02

Beads: `gohawk-dho.4.3`. This is a new snapshot after consolidation through
`707913f`, preserving the historical labels and 22-site replay ledger.
The [site ledger](remaining-eleven-reassessment-2026-10-02.tsv) records all
eleven original position/check keys in seven pinned package scopes. Every
scan succeeds with exit zero and empty stderr. Eight sites remain visible;
three are absent with exhausted evidence. All eleven remain semantically
unresolved in this snapshot. Rune is a separate open investigation.

## Reassessment

| Family | Sites | Current evidence and next requirement |
| --- | ---: | --- |
| Skywalking lock/field association | 2 | Both read-lock writes remain, each emitted twice. Cursor fields and the event list have different uses; unlocked cursor writes do not prove confinement. Needs exact guard/field or participant evidence. |
| Debian producer/consumer ordering | 2 | Both workers remain. Dynamic production, group registration and consumer progress require a shared participant/closure relation; nearby waits or guessed loop counts are insufficient. |
| Openase transport completion | 2 | Both are absent. Each target records three budget-exhausted events before an opaque-ownership-transfer unknown decision. Interface-returned readers, session ownership and deferred Close remain unresolved; silence is not credited as a cleanup proof. |
| boxesandglue caller exit | 1 | The uncovered helper return remains. Error-triggered exit belongs to its caller; the helper cannot publish an unconditional release fact. |
| goiardi caller preconditions | 2 | Both SQL helper findings remain. Caller configuration conditions need stability through intervening calls; mutable global flags cannot become universal callee guarantees. |
| Ferro constructor-dependent Start | 1 | Absent with three budget-exhausted events and no final process decision. Pre-start owner collection encounters configureProcGroup's void result; skipping it in an ignored prototype moves exhaustion to StdinPipe's tuple result. Neither is a semantic fix. NewIO's empty command state is still needed to exclude the transport Start error branch. |
| rev-dep detached telemetry | 1 | Remains visible. Actual SSA shows local Write/Close on the exact StdinPipe result. Those operations and their results provide no wait owner; investigate the existing unused-command unknown boundary rather than inventing a detachment guarantee. Follow-up: `gohawk-dho.4.4`. |

These are evidence requirements, not declarations that the families are
unmodelable. The existing pinned source anchors are in the
[earlier family assessment](remaining-fp-assessment-2026-10-01.md).
Target resolution alone still has no demonstrated correction. Openase needs
reader/session relationships and transport completion even with resolved
methods; Ferro needs receiver-state-dependent result feasibility.

## Receipts and limits

Immutable binary `.build/goal-cache-removal-reviewed`, SHA-256
`c30d2d011ae4e9d6ac45f4d91a566aa81fbf1c768c8d19473f612385ac5cb898`.
Metadata `.build/goal-eleven-current/scans.json`; individual JSON receipts are
named in the ledger. Scans use all checks, readonly modules, disabled CGO and
workspace mode, 180-second limits and at most two simultaneous package scans.
Ferro uses `./mcp`, the scope containing the remaining site; its unrelated
registry finding is outside these fixed eleven keys. Multiplicity is retained
separately from location counts.

Actual SSA and targeted evidence traces for Openase and Ferro are saved under
`.build/goal-eleven-current/`. Target event extracts are
`openase-target-events.json` and `ferro-stdio-events.json`. Rev-dep's SSA is
`revdep.ssa`. The ignored void-result prototype and broad pipe-result prototype
are investigation receipts only. The latter suppresses rev-dep but also loses
the returned-pipe fixture, so that version is rejected.

Graph tools were unavailable; source fallback inspected the selected pinned
families and current process proof. This is a scoped precision reassessment,
not a full corpus replay, call-graph benchmark or completed architecture audit.
