# Closed phase domains

Beads: `gohawk-dho.44.14`. Parent source: `d2864432`.

Goroutine discovery and lifetime cutoff helpers now accept numeric query phases.
The shared tracer's event and diagnostic phases use numeric `trace.Phase`.
Check/reporting, analyzer wrappers, CLI selection and golangci emit numeric
phases; serialization converts them to the same text labels in `trace.Record`.
The two domains remain distinct: query phases describe a budget request in
trace details; event phases describe the role of a record. No diagnostic proof,
budget constant, wire field or label changes.

The architecture syntax guard rejects raw phase parameters. The typed guard
resolves their aliases and named Phase classification domains. Matcher controls
include direct and imported text, numeric phase parameters and serialized phase
fields. Syntax and typed guards share parameter-selection mechanics. The first
lint run rejected complexity growth in the two matchers; extracting that shared
selection fixes it without exemptions or higher thresholds.

Full shared trace/check tests pass. The focused analyzer/architecture selection
passes; its initial trace regex matched no trace tests, so that output is not
credited as trace validation. `TestProbePhaseLabels` checks JSONL emission for
all five event methods, preserving reason and unknown outcome alongside the
expected phase label. Existing trace tests cover disabled selection, candidate
attribution, concurrent emission and diagnostic events.

Fourteen read-only all-check parent/current scans exit zero with empty stderr.
All complete payloads agree: lock116, goroutine128, cancellation40, resource318,
process41, pinned stargz1 and pinned goiardi9, totaling653 diagnostics. The two
residual goiardi sites persist. Exact scopes, pins and exits are recorded in
`.build/goal-phase-enums-scans/scans.json`; `comparison.json` records the full
payload comparisons.

Two additional read-only goroutine fixture scans enable tracing with JSON
output. Both exit zero with empty stderr. Their diagnostic payloads match the
corresponding untraced runs. All3,359 trace records agree as complete record
multisets across the parent/current binaries; accepted, observed, rejected and
unknown outcomes remain represented. Inter-process event ordering is not
claimed. Receipt: `.build/goal-phase-enums-traces/comparison.json`.

Reviewed SHA-256:
`5a4a7a46c8c3207437a5c4df93f45b7b304af3cff04c4667d0fde2ffa3f440d4`.
It matches the canonical executable; these are precommit artifacts.
The normalized scan covers338 production files and2,210 declarations, retaining
five full-body and33 partial groups with identical path/name/token signatures.
No semantic absence claim follows from that scan. Graph tools remain
unavailable; exact source fallback was used.

No full precision replay, local race run or production FP correction credit.
The final goroutine proof inventory and five production FP sites remain open.

Final `make verify VERIFY_TIMINGS=1` passes all eight targets after the matcher
extraction, including ordinary tests, static checks, generation and self-dogfood.
Receipt: `.build/goal-phase-enums-final-verify.log`. The earlier lint-failing run
is retained in `.build/goal-phase-enums-verify.log` and receives no passing credit.
Final architecture validation after the maintained docs passes; receipt:
`.build/goal-phase-enums-architecture.log`.
