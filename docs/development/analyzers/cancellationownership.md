# cancellationownership design notes

The public page is [cancellationownership](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

For a standard context derived directly from a fresh local cancelable context,
the classifier also considers use of the exact parent's cancel function after
the child is created. Parent cancellation or ownership handoff makes child
cleanup uncertain rather than proving a leak. This reuses the existing
path-sensitive classification; an unrelated or only conditionally canceled
parent does not cover an otherwise unowned return. A parent held in a local
cell is resolved at the child-creation instruction, including when that cell
later holds the child. Ambiguous or detached parents remain outside this
boundary. Signal registrations still need their own stop
function even when their parent is canceled.

Receiving the exact standard context child's `Done` channel declines a loss
claim on that path. In a `select`, this applies only to returns dominated by
the selected receive arm; other cases and default arms still need cleanup.
This is uncertainty about outstanding cleanup, not proof of a synchronous
cancel call. Signal contexts still require unregistration after a signal.

A helper that calls the cancel function only behind a Boolean parameter
settles it at a call whose constant argument selects the calling branch,
through the same argument binding and summary cases as `resourcelifetime`.
For an imported helper, whose body is not visible, the classifier asks the
lifecycle evidence once the body search is undecided: a summary that calls
the exact cancel function synchronously, on every return or in the case the
constant arguments select, is a release. Only a proof counts. A helper that
calls it on another goroutine, or calls a different argument, stays unknown.
A local helper called with the constant that skips the call is reported. An
imported one is not: summary cases carry only positive guarantees, so a
summary that does not claim the call is not proof that the helper never
cancels, and the call stays unknown. Fixtures:
`cancellationownership/argument_cases.go`.

Local callback invocation uses the same structured completion engine as
method cleanup, including exact identity, stable capture mapping, callback
bindings and bounded recursion. The Boolean adapter projects only a proven
result; unavailable or recursively rejected bodies remain unknown to the
underlying proof. Invocation on another goroutine never establishes a
synchronous release. Shared tests in `lifecycle/completion_invocation_test.go`
cover replaced and mixed callbacks, conditional invocation, bound invokers,
asynchronous launches, recursion and budget exhaustion.

A deferred literal that captures the cancel function is judged exactly when
the capture is simple: the cancel function is stored once into a cell that
only directly deferred literals read, each deferred after the store. Such a
literal that calls cancel on every return releases it where it is deferred.
One whose call turns on a named result, as `defer func() { if err != nil {
cancel() } }()` does, settles nothing at the defer; each return it dominates
asks the shared completion search (`lifecycle.ProveResultGuards`) whether the
literal calls cancel given the value that return stores, so a success path
returning nil without cancelling is reported, as grpc-go's ALTS handshake
was before its fix. Any other capture keeps the store an opaque use: a cell
the function also reads (a call through the loaded variable is not
recognized), a literal deferred before the store (the walk never meets it),
or one launched or handed to a callee. Fixtures:
`cancellationownership/result_guarded_defers.go`.

Literal outcomes are checked before asking the summary broker. An unconditional
result guarantee is projected by `resultfacts.Guarantee.Outcome`, shared with
resource lifetime; unknown guarantees cannot select the cleanup branch. The
classifier retains its own budget and absent-provider boundary.

Ordinary return uses, result-guarded deferred cleanup and returned owners are
combined into one cached instruction label. Exact cleanup or transfer covers
an otherwise opaque return use; uncertainty never becomes exact cleanup.
The ordinary obligation flow consumes that label without a second return
callback. A merged return can be visited under several flow states, but its
instruction-local evidence is queried and traced only on the first visit.
`cancelledOnMergedSuccess` in the result-guarded fixture pins one cleanup label
and one accepted final decision; the direct returned-cancel fixture pins an
accepted transfer label rather than the former unknown ordinary-use label.

Elapsed sleep durations remain a known precision gap, not a timer exemption.

### One-time program-entry contexts

A standard context constructor directly in the true program entry, outside a
control-flow cycle, may supply process-lifetime context state. At each normal
return the existing classifier labels this as `unknown`, with reason
`process-lifetime-context`; the ordinary obligation walk still decides the final
outcome. Exact deferred cancellation or transfer can retain a stronger outcome.
This is a precision boundary for bounded retention at one acquisition site, not
proof that the cancel runs, children are canceled, or workers are joined.

The classifier reuses `ssaflow.RunsOnceInProgramEntry`: the function must be the
package-scope `main` in package `main`, with no reference to it anywhere in the
package. Loops, helpers, closures, methods, and functions called `main` in other
packages remain checked. `signal.NotifyContext` stays outside this boundary:
unregistering a signal handler changes process behavior during the lifetime,
not only context retention. Facts and reusable callee guarantees are unchanged.

This narrows the check and accepts missing diagnostics when work finishes before
process exit but its directly acquired entry context remains uncanceled. It also
applies to standard deadline/cause constructors; their cancellation is never
inferred merely from this boundary. Contexts created in called helpers do not
inherit it. The representative source is
[k8ssandra's context and conditional handoff](https://github.com/k8ssandra/k8ssandra-operator/blob/2028d352ecb495de4b6e053d99d7a77b21eb5107/main.go#L176-L205).

Fixtures `entrycontext/main.go`, `callableentry/main.go`, and
`namedentry/main.go` cover the accepted entry context and cause form, an exact
deferred timeout cancel, and the excluded loop/helper/closure/method/signal and
referenced/non-entry main forms. The analyzer's `program_entry_test.go` checks
final outcomes and one unknown label per return, including when exact cleanup wins.
The small policy extension stays at the existing classifier decision point in
`proof.go`; it adds no traversal or parallel reporting rule to that already
large cohesive proof implementation.

### Cancels owned by a returned struct

A constructor commonly stores its cancel in a field of the struct it
returns, directly or as a closure that calls it, and returns early with an
error before that struct reaches the caller. When the struct is allocated in
this function, the cancel reaches exactly one of its fields, and the struct is
used only through field addresses and returns, the struct owns the cancel:
returning it is a transfer, and a return that drops it leaves the cancel
uncalled. A closure qualifies when it captures a cell written once with the
cancel, initialized before capture (`ssaflow.WrittenOnceCellAtWithin`), and
is stored straight into the field. Any
other use of the struct, such as passing it to a function, publishing it, or
capturing it, keeps the ordinary classification and the obligation unknown;
so do a cell written twice, written after capture, or read before the store,
and a closure handed to opaque code. Reading the cancel back out of the
field is not an exact release. Fixtures:
`cancellationownership/owner_structs.go`.

Of the recall audit's captured-cell misses (stage 3), grpc-go's
`ClientHandshake` is now reported through the result-guarded defer rule, not
this one. go-test's `prepareTestRun` hands the cancel to goroutines that call
it on failure, which cannot be ordered against the function's returns, so it
stays unknown. hermesx drops its cancel on an error return before the owning
struct exists, after passing the context to other components, which is also
unknown.

## Result-guard discovery availability

The shared result-guard census now returns `lifecycle.ResultGuardsProof`.
Instruction, capture, named-result and opposing-completion queries consume the
same request child. Interrupted discovery publishes no guards and stops the
cancellation proof at unknown before the ordinary obligation walk; it cannot
turn an omitted result guard into release or loss. Ordinary opaque completion
answers retain the existing modeled-guard policy.

`result_guard_budget_test.go` compares actual-SSA cancel-on-error loss,
cancel-on-success release, and a larger function whose discovery exhausts the
1,000-step child while the candidate pool remains available. Shared lifecycle
controls exercise multiple guards, partial-list cutoff and fresh recovery.
Capture-filter and per-return value/outcome queries retain their existing
policies and costs; this change bounds discovery rather than claiming that all
cancellation evidence is now transitively bounded.

### Result-guard return allowance

Return-specific cleanup shares one query allowance across guard registration,
last-store selection, returned-value outcomes and deferred completion.
`ResultGuard.ProveReachesReturn` owns the common dominance/possible-follow
policy: a possibly registered defer or interrupted search is unknown, while a
completed disconnected search contributes no cleanup. Both lifecycle consumers
use this proof rather than reproducing its ordering decision.

`ValueAtReturnWithin` retains the exact named cell's last store before
`RunDefers` in the return block. Earlier-block assignments and recovery returns
without such a store remain unknown; another cell cannot establish the result.
Literal outcomes precede summary lookup, and imported outcomes spend the same
return query allowance. Cutoffs cannot turn a missing binding into a skipped
cleanup or a completed cleanup. The shared
`completion_result_return_budget_test.go` pins actual SSA for multiple results,
multiple returns, overwritten stores, earlier-block assignments, conditional
registration and interrupted callbacks. Consumer `result_guard_budget_test.go`
checks cutoff unknown followed by fresh release and skipped-cleanup answers.

### Deferred capture filter allowance

`proveDeferredCaptureCellWithin` is the structured filter used both for a
cancel's store and when selecting captured result guards. One allowance covers
closure bindings, cell referrers, nested lexical read-only checks and exact
store-before-defer order. `WrittenOnceCellWithin` shares the old cell identity
engine; `ReferrersWithin` supplies bounded use enumeration. The completed
policy still requires one exact store and only directly deferred readers.
Loaded calls, other readers, multiple or nested writes, early registration,
launches and handoffs stay outside that proof.

Capture cutoff cannot make a store transparent or discard a result guard as
absent. It produces unknown before flow setup or an unknown instruction label.
A deferred literal shares its capture allowance with guard selection and
completion. Existing label reasons and the authoritative flow remain in use.
`deferred_capture_budget_test.go` covers nine actual-SSA capture families,
wrong targets, child/parent/fresh queries, classifier cutoff and four full
cancellation outcomes. Shared cell tests cover rejection causes and a large
nested reader whose child expires while its parent remains available. Other
default once-stored-cell consumers remain tracked separately in
`gohawk-dho.44.11.5.25`; no whole-cancellation cost bound is claimed here.


## Constructor-owner availability

Constructor owner discovery returns one structured proof. Its cancel, cell,
closure, owner and sibling-field referrer censuses share one classifier child
and the candidate-wide pool. Shared once-store and dominance evidence uses
`ssaflow.WrittenOnceCellAtWithin` at closure creation; no separate analyzer
store-order check remains. The completed owner-use census also names exact
owner returns, so return classification reuses it instead of scanning results.

Cutoff drops every partial hold and supplies `owner-evidence-unavailable`
unknown labels. A complete missing owner remains ordinary classification.
Exact direct cleanup and result-guarded or directly returned cancellation retain
precedence over unavailable owner evidence. The proof is cached only for the
current classifier, including its stopping reason; a fresh candidate has a new
classifier and allowance. Actual SSA contract, child/parent/fresh, padded capture,
ordering and sibling-field controls, and flow cut/cleanup precedence are in
[owner_budget_test.go](../../../internal/analyzers/resources/cancellationownership/owner_budget_test.go).
No package or callee guarantee is inferred from this caller-local owner proof.
