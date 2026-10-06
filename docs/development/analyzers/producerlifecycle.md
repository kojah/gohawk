# producerlifecycle design notes

The public page is [producerlifecycle](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

Complete shared concurrency summaries expose sends and receives hidden in
helpers, including imported helpers. They feed the existing bounded count proof:
a local unbuffered channel, an observed receiver, and explicit excess sends.
Deferred receiving helpers count as receivers too. Opaque channel consumers,
possible drain loops, and additional receiving goroutines leave the result
unknown rather than being mistaken for absent receivers. Incompletely modeled
competing workers can therefore suppress a finding, unless shared call-effects
evidence proves that all uses of the channel are non-receiving. Buffered channels and
externally supplied channels remain outside this proof.

Equivalent branches in a complete concurrency summary describe one ordered
operation. Its alternate source positions are diagnostic attribution only;
they must not multiply the send count. `helpers/branch_sources.go` covers a
balanced branch send, balanced pairs, a true excess second send, and distinct
competing workers. The count proof runs once per operation; reporting and
tracing retain each branch position.

Distinct contributing launches must form a dominance chain before their sends
can share a total. Unordered launches leave the count unknown, including
mutually exclusive branches followed by a common worker: each branch can
reach the common worker without both alternatives executing. This deliberately
misses some real excess production in independent conditional branches.
`helpers/launch_choices.go` pairs those accepted forms with serial and nested
ordered-worker diagnostics. The most recent launch is the dominance frontier;
all earlier members are its ordered ancestors. Shared instruction dominance
also orders launches in the same block.

When a complete worker summary is unavailable, contributing direct sends must
also form a dominance chain within that worker. Alternative sends can each
reach a common later send without both executing. The same frontier mechanic
orders launches and direct sends; `countProducerSends` returns a structured
count proof consumed by the final receiver comparison. Complete summary
operations retain their normalized sequence ordering. `helpers/fallback_choices.go`
covers a balanced branching worker, competition with another worker, and
straight-line/nested excess-send diagnostics. This deliberately leaves some
real excess protocols with unordered fallback sends unknown.

Receiver helper summaries and channel-identity queries share an allowance.
The incomplete-worker non-receiving-use proof shares that allowance too.
A cutoff at any of these steps yields `receiver-budget-exhausted`, discarding even
an earlier partial receive count. A complete summary alone cannot prove an
absent receive after identity resolution stops. `receiver_budget_test.go`
checks cold and warm one/two-receive helpers and non-receiving workers across
every allowance through
first completion. Normal complete and asynchronous receiver policies remain
separate from this cutoff boundary.

Opaque callback choices retain receiver uncertainty when any selected closure
captures the channel. The shared lifecycle closure-capture query follows phi
alternatives, retains opaque conversions/loads and shares the classifier's
allowance. A positive capture cannot count as a receive or prove invocation.
`helpers/selected_drains.go` pairs balanced selected drains with unrelated
callback choices that retain the excess-send diagnostic; trace assertions pin
the unknown helper reason. The process owner uses the same capture mechanic
and keeps its own unknown Wait-participation interpretation.

Loop-based send counts remain unknown: a repeated statement does not prove
multiple sends are feasible. This deliberately misses unbounded producer loops
until their excess production can be established without a cardinality guess.
A `go` statement inside a loop counts the same way: it starts as many
producers as the loop runs, which may be none or one, so its sends are an
unknown count even when the goroutine body sends once. Before this rule such a
producer was counted once and traced as within the receive count
(`producerPerItem` in `uncertain_counts.go`).
Callers that terminate the process instead of returning do not establish an
abandoned receiver lifecycle.

## Retired experimental checks

Four experimental checks were removed on 2026-09-27; the last revision
containing them is `73f25e27`. The experimental tier itself remains.

- `stopped-loop-send` reported a send to a struct-owned channel after the
  service loop serving it had returned, and `unclosed-range` a range over a
  channel whose producer could return an error without closing it. Neither
  made a report across the batch 56 to 61 audits, about 1,500 repositories.
- `unreceived-return` reported a one-shot worker blocked on an unbuffered send
  the launching function could return without receiving, and
  `unsignalled-receiver` a worker blocked on a receive the function could
  return without satisfying. In batch 61 the first made eight reports, all
  true positives, seven of them also reported by `goroutineownership/unjoined`;
  the second made none.

The channel census they shared was removed with them.

## Final diagnostic trace projection

Final reportability evidence is presented through `trace.DiagnosticOutcome`.
Proof rules and report gates remain local; shared presentation neither supplies
missing evidence nor turns uncertainty into a diagnostic. Existing trace fixtures
pin the supported outcomes and source attribution.


## Direct fallback callee resolution

Incomplete protocol summaries use `calls.DirectCallee` to select only a
statically named function or literal closure. Generic instances are resolved to
the source origin before sends are collected; parameter positions and closure
captures then map back through the existing `SpawnedValueAtCall` query. The
instantiation wrapper may contain only a forwarding call and therefore cannot
stand in for the source body's send instructions. Dynamic function parameters
and phi-selected closures remain opaque; this adds no callback-choice inference.

`callee_test.go` checks actual SSA for direct, captured, generic and balanced
generic workers, plus unresolved dynamic and selected workers. The origin's
send instructions must bind to the caller's channel, and the existing count
proof rejects excess production while accepting the balanced form.
`helpers/generic_fallback.go` pairs accepted and diagnostic source fixtures.
Ordering, repeated-send, local unbuffered-channel and receiver-uncertainty rules
are unchanged. This change follows a source consolidation review rather than a
production FP; it earns no FP correction credit.


## Non-receiving participant binding allowance

`nonReceivingUses` yields `CallBindingsWithin` under the same allowance as its
channel/cell effect proof. Interrupted metadata retains unknown at the existing
final availability fence; it cannot prove a participant has no receive. Exact
captured-channel matching, possible containing-owner policy and read-only cell
validation retain their existing independent query costs. Completed channel
mutation effects establish this narrow absence claim, not invocation or joining.

`binding_budget_test.go` compiles a worker with 66 supplied arguments, an opaque
unrelated callback and one channel send. A 32-visit child cuts metadata and
returns unknown while its pool stays usable; a fresh child completes. The parent
instead returned complete non-receiving evidence at that cutoff. Existing
`receiver_budget_test.go` controls retain cold/warm summary admission and exact
receive counts for ordinary helpers and incomplete sending workers.
