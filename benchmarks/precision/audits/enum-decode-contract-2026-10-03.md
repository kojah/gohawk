# Shared enum decode update contract

Bead: `gohawk-dho.23.21.9`. Parent source: `ff0e35f`.

The refreshed normalized whole-function scan found identical decode-and-assign
bodies in catalog kind/tier, SSA proof provenance and trace outcome adapters.
All four decode a label and assign only on success. This state-update rule now
belongs to `enumtext.Decode`; adapters supply their typed destination and domain
labels. Unknown input preserves previous values. Valid labels and unset zero
labels are unchanged. Nil destinations return an error rather than dereferencing
an unavailable receiver. The numeric enum types and public text methods remain
unchanged; no analyzer acceptance rule is modified.

Existing domain controls round-trip all 15 zero/named labels through JSON and
reject unknown labels and numeric wire input without overwriting the receiver.
All four invalid numeric domain values remain rejected on marshal. A nil codec
destination has focused coverage. Enumtext, catalog, SSA, trace and public
analyzer package tests pass.

`make verify VERIFY_TIMINGS=1` passes all eight gates: generate 3s, modules 1s,
vet 3s, formatting 3s, deadcode 8s, lint 23s, dogfood 62s and tests 109s.
Ten CLI parent/current comparisons preserve exact exit status, stdout and
stderr, including catalog lists, selection validation, tier errors and help.
Domain JSON controls cover serialized enum labels directly. No full precision
replay, external repository scan or local race run was needed for this codec
change. No FP credit is claimed.

Reviewed/canonical binary SHA-256:
`f5b7076df7f42251620adb01a7c0020b8bb769614b4dbbadd7fecdda12e63fea`.
Parent SHA-256:
`85a1bf572e5f0ce210f5428f1dabaf4b79f940cd43a11ed4907582c20528ba90`.
Local artifacts use `.build/goal-enum-decode-*`.

## Refreshed duplicate candidates

Exact-source fallback was used because graph/index tools were unavailable.
The scanner definitions and exclusions remain those described in the completion
audit: complete bodies of at least 35 normalized tokens and adjacent windows of
three through six statements with at least 50 tokens. Identifier normalization
also hides types and predeclared values. These are review candidates, not a
semantic absence proof.

At parent source the scan covers 334 production files and 2,201 declarations,
with six whole-function groups and 35 partial groups. After this change the
whole-function groups fall to five; the same 35 partial groups remain. Counts
are overlapping syntactic candidates, not a defect metric.

The five remaining whole-body groups retain their different contracts:
carried-resource versus guarded-body proof results (proven versus unknown),
domain classifier caches, destination-specific store evidence, candidate-budget
adapters with different limits/attribution, and resource-presence versus stored
value constructors. Exact current source confirms those distinctions.

The current partial indices below are based on the parent scan; removing the
whole-body decode adapters does not change this partial inventory.

| Partial indices | Current source disposition |
| --- | --- |
| 0 | Typed instruction inventories append calls, defers, returns and branches; one body traversal. |
| 1, 2, 4, 10, 13, 14, 15 | Goroutine proof branches preserve distinct reason/outcome and policy rationale; one authoritative proof. |
| 3 | Diagnostic source-position ordering differs from heap root/slot ordering. |
| 5, 34 | Process-use and returned-ownership result adapters have different search/exhaustion owners; the matched proof-construction scaffolding is not a second search. |
| 6, 16, 20 | Fresh allocation/field uncertainty differs from merged phi cleanup arguments. |
| 7, 32 | Resource success delegates ordinary nil comparison to the SSA helper; selected-receive edge evidence has different conditions. |
| 8 | Intentional CLI scaffolding retains check/analyzer/group overlap and error contracts, as previously reviewed. |
| 9, 22, 24, 29 | Concrete SSA forms select distinct operands, effects or transparency permissions within existing shared dispatchers. |
| 11 | Selected arm decoding differs from guard identity/contradiction extension. |
| 12 | Resource assertion uses possible storage/derivation to remove a failed arm; concrete-type assumptions require structural identity and a supplied type. |
| 17 | Possible deferred capture differs from every matching capture having a locally resolved use. |
| 18 | Carried-resource proof is positive; guarded-body evidence is uncertain. |
| 19 | Stored-path search differs from exact value-at-path lookup; both share the graph state/content engine. |
| 21 | Immediate process nilness and counted induction headers have different observation and comparison contracts. |
| 23, 33 | Classifier adapters cache/trace distinct domain actions from their own single classifier. |
| 25, 28 | Comparison shape precedes distinct select-index versus error/Boolean result semantics. |
| 26 | Field/global stores use possible alias; enclosing callback storage uses derivation and a different destination. |
| 27 | Explicit slice clones preserve summary schema and nested mutation isolation. |
| 30 | Stable callback content agreement is duplicated within field/element resolution. Follow-up `.23.21.10` extracts that rule while keeping geometry separate. |
| 31 | Callback array shape differs from readable captured channel/synchronization/cancel storage. |

An additional source finding outside normalized windows repeats the selected
fresh lock instance identity during binding; `.23.21.11` tracks reusing it while
retaining the class-only helper's separate caller. These two concrete findings
keep the broader semantic parent open. The five-site production FP queue also
remains open. This refresh does not claim overall architecture completion.
