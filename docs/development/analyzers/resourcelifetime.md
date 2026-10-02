# resourcelifetime design notes

The public page is [resourcelifetime](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Final decision and tracing

`evaluateResourceFlow` owns the final diagnostic evidence and reason, including
its memory-writer policy exclusion. Reporting checks only the proven state;
tracing projects the same state with `trace.DiagnosticOutcome`. The state is
about permission to report, not proof that a resource was closed: exact release,
unreachable acquisition and policy exclusions suppress the diagnostic with a
disproven result. Opaque consumption, HTTP acquisition uncertainty, possible
pre-acquisition deferred release and unavailable instruction evidence remain
unknown. The reporter does not reconstruct this distinction from reason codes.

The existing memory-writer check still precedes candidate evidence and all flow
queries. It now yields a final accepted policy decision instead of bypassing
that boundary. Its reason explicitly excludes external-resource ownership;
unfinalized compressed data can still be a defect outside this check.
`assertResourceDecisions` shares the ordinary fixture run and covers exactly
one final decision for selected opaque/imported, deferred-release, memory-only,
mixed-writer and reportable controls. No reporting policy, traversal, cleanup
contract, query budget or exported summary schema changes with this projection.

Helper cleanup labels preserve completion uncertainty: a budget-exhausted
search yields `unknown/budget-exhausted`, not settled cleanup. Both suppress
the leak report, but only proven cleanup or transfer discharges the obligation.
Loop-only completion retains its existing unknown boundary; a conditional
helper with a complete uncovered return keeps the ordinary classification.
The pre-acquisition deferred-cleanup query remains a may-release boundary and
still declines a diagnostic on exhaustion. The focused budget test uses actual
SSA completion queries for exhausted, exact, conditional and loop-only cases.

Ordinary helper completion draws each method query from the candidate pool,
with its existing 250,000-step allowance. Storage retains its separate smaller
query allowance; completion must not inherit that cap. Exhaustion reaches the
pool's observer and remains unknown. The classifier budget regression exercises
an exhausted pool through the real helper classification and checks exactly one
method-completion give-up at the call. A helper with more than the storage limit
in actual SSA remains provably cleaned up. The pre-acquisition deferred query
still has its own bounded allowance; this change does not move that boundary.

The resource coverage walk now shares the existing candidate pool for its
acquisition index and reachability, dominating guards, queued states, guard
keys and invalidation, termination summaries, and successor guard extension.
`proveResourceFlow` owns one structured outcome and witness. Exhaustion of
either the walk child or a sibling query's parent pool yields unknown and
discards any tentative leak witness; it cannot establish release or absence
of an acquisition. Optional acquisition and error-edge activation retain their
existing policy. `flow_budget_test.go` exercises actual SSA cleanup and leak
paths, including dominating guards, at every insufficient allowance.

The existing bounded guard and successor engines are exposed for this custom
resource state machine; the generic obligation walk uses those same engines.
Termination-summary inference receives the walk allowance, and a truncated
literal feasibility query retains all successors. Pre-acquisition policy and
ownership queries, resource-specific error predicates, heap graph
construction, type-system internals and custom library contracts retain
independent costs. This is not a whole-query wall-clock bound.

Resource presence now returns one structured proof from `flow_presence.go`.
Incoming-phi selection, nil evidence, assertion storage identity and possible
derivation spend the flow allowance. Only a completed presence proof can remove
an obligation on its absent arm. Assertion and nil-comparison policies remain
unchanged; possible derivation is not strengthened into exact identity.
`flow_presence_test.go` checks both arms, reversed nil comparisons, assertions,
unrelated values and every insufficient allowance on actual SSA.

The shared `ssaflow.DerivesFromWithin` engine charges queued values, arbitrary
operands, load/store referrers and aggregate-address use scans. Heap derivation
adds an alias-dispatch charge without bounding graph construction or the alias
query's internals. Default callers retain their existing unbounded traversal.
`value_derivation_budget_test.go` checks calls, stores, nested whole-aggregate
loads, replaced fields, cycles and callback cutoff. The remaining resource cost
families are pre-acquisition evidence and returned-owner evidence; those still
need separate review rather than a whole-query bound claim.

Scoped resource controls for Beads `gohawk-dho.44.11.5.16` use the immutable
`.build/goal-resource-presence-current` binary, SHA-256
`391b8283a29e4ab972edf3babac837fcffada2d9c1619b462bffb4dd1e0e75a8`.
Cute at `9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3`, scope `./...`, retains
its reviewed TP (exit 3, 980-byte JSON). Ferro at
`d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4`, scopes
`./internal/admin/repository ./mcp`, retains the corrected FP's absence
(exit 0, `{}`). Both use `-enable=resourcelifetime -json`, `CGO_ENABLED=0`,
`GOFLAGS=-mod=readonly`, `GOWORK=off`; stderr is empty and JSON is identical
to the previous resource-flow binary. These are scoped controls, not a corpus
replay or new verdicts. An unbounded derivation overlay fails all five positive
allowance controls; fresh queries and sibling-pool cutoff controls pass.

## Detection boundaries

Release owned resources on every path. Storing a resource in a partially
constructed object does not transfer ownership when an error path returns
without that object.

The built-in contracts cover files, transactions, SQL rows and statements,
HTTP response bodies, and gzip/zlib writers. Compression readers do not own
their inputs, and their Close does not finalize or validate the input stream,
so they are not cleanup obligations. The underlying file still needs closing.

HTTP bodies come from `http.Get`, `http.Post`, `http.PostForm`, and the
`Client` methods `Do`, `Get`, `Post`, and `PostForm`: net/http documents
"Caller should close resp.Body" for each. `Head` in either form carries no
such sentence and usually returns `http.NoBody`, so it is not an acquisition.
A project type named `Client` with a `Get` method is matched by package path,
not name, and is not an acquisition (`http_client_methods.go`).

A direct `http.NewRequest("HEAD", ...)` used only by `Client.Do`, with a
fresh zero-value client also used only by `Do`, has an uncertain body
acquisition. The usual transport returns `http.NoBody`; the replaceable global
transport prevents treating this as proof of no resource. Explicit client
timeouts, custom transports, request mutation, and helper escapes are outside
this narrow boundary and retain the ordinary cleanup obligation.

An exact `http.Get` targeting an unchanged local `httptest.NewServer` can also
have uncertain body acquisition when its handler and visible helpers only set
non-framing headers, cookies, or a non-redirect status. Writer escapes, body
writes, dynamic or framing headers, and visible default-client or transport
overrides exclude this boundary. `server.Client().Get(...)` on the same
server qualifies too: httptest gives that client a transport to the server
and no timeout. Any other use of that client, such as setting `Timeout`,
and any other client, exclude it (`http_servers.go`). Mutations of global HTTP defaults hidden in
other packages remain an accepted coverage gap; this is not proof that a body
can never need closing.

A returned wrapper that holds the resource, proven by the `ReturnedOwner`
summary of every constructor in the chain, as `log.New`, `slog.New`, and
`slog.NewTextHandler` hold their argument, is a handover when the constructor's
own summary claims a retaining result; the caller then owes the resource and
is reported where it drops the wrapper. A proven chain without that claim,
including any unexported constructor, and a wrapper returned inside an
aggregate, are uncertain boundaries. Discarding the wrapper on an error return
still abandons the resource. See `retaining_results.go` in the fixtures.

Asynchronous resource use through a helper supplies unknown ownership locally
and across package boundaries. The local call-effect query and the imported
`ClaimAsynchronouslyExposes` selector feed one classifier predicate. The latter
derives from existing heap escape effects for the exact parameter, not from
generic retention or child-field effects. It does not prove release, transfer,
or which return paths launch the work. A synchronous writer, exposure of another
argument, and a return bypassing the handoff retain diagnostics
(`imported_async.go`); heap-claim tests cover child fields and missing facts.
[Viewcore's profiling writer](https://github.com/golang/debug/blob/ac862fd6552b739f50ba812382eed75745a129b1/cmd/viewcore/main.go#L820-L829)
is the representative imported handoff. Conditional exposure can hide a real
leak on a callee path with no launch, matching the existing local may-exposure
boundary. This extends the existing asynchronous-consumption family in the
large classifier file; policy and tracing continue through its single label.

For compression writers, error returns and explicit pipe aborts can abandon
the output rather than publish it. Those paths are uncertain, not proven
finalization; successful returns still require Close where the contract applies.
An error interface holding a typed nil pointer is nonnil, so it also supplies
uncertain abandonment on an error return or `PipeWriter.CloseWithError`.
`compression_error_boxing.go` pairs those forms with an unfinished successful
return whose nil error still leaves the finalization obligation uncovered.

The false edge of the tracked `database/sql.Rows.Next` call is also uncertain:
the final result set closes automatically, but another result set may remain.
An early break or return before exhaustion is not covered by that boundary.
By contrast, `Rows.NextResultSet` returning false closes the exact Rows value.
This result-conditioned release can pass through a forwarding helper and its
cross-package lifecycle summary, or a visible straight-line owner method that
forwards the call through an exact field. The field may hold Rows through an
interface only when its concrete value is proven to be that same `*sql.Rows`.
A true result, another field or Rows value, opaque dispatch, and a wrapper
whose field relationship is unavailable do not prove release.

Closing the exact parent DB, directly or through a dominating defer, suppresses
missing-release findings for DB-prepared statements. It does not discharge
rows, transactions, or statements prepared through a Conn. Parent closure is
not treated as immediate statement invalidation.

Committing or rolling back the exact parent transaction also makes active row
cleanup uncertain: transaction-context cancellation closes its rows, including
rows queried through a statement prepared on that transaction. A dominating
deferred finish is recognized; a different transaction or a finish confined
to only some return paths is not. This does not treat `DB.Close` or
`Stmt.Close` as closing active rows.

Passing a wrapper around an aggregate that contains the exact resource to a
callee that may retain it can make ownership uncertain, not prove cleanup.
This includes `io.MultiWriter` installed as logging output. The wrapper must
itself potentially retain the aggregate; an ignored input or read-only
transformation does not establish this boundary. Discarding the wrapper does
not by itself transfer ownership.

A longer chain of constructors that each keep their argument, such as
`slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, file), nil))`, hands the
resource over only where a callee is proven to store it: `slog.SetDefault`
installing the logger as the process default, for example. Storing such a
chain into a field or element of an object the function did not allocate, or
into a package variable, such as routing a logger into a server's `ErrorLog`,
hands it to that object. A chain the function only uses, such as a logger
whose `Info` it calls, or one stored into a local allocation, leaves the
resource owed. Up to four constructors are followed.

The same bounded chain proof follows explicitly appended wrapper values through
the compiler's variadic array when the resulting slice is stored on a foreign
owner. It reuses `ssaflow.AppendedValues`; spread slices remain outside this
query. This covers [KCL's logger options](https://github.com/twmb/kcl/blob/5290cb05bcc421a239e327ba11408bc4e27bd2dd/client/client.go#L1445-L1456)
without inferring ownership from logging names or process lifetime. An unrelated
wrapper leaves the obligation live. A discarded local slice containing wrapped
resources remains an accepted false-negative gap: the earlier append can become
opaque consumption, so this publication query is not a local collection proof.
`resourcelifetime/published_wrapper.go` pins these distinctions.

The returned-wrapper gap was isolated in the
[urunc assessment](../../../benchmarks/precision/audits/returned-logger-assessment-2026-10-01.md).
The shared heap projection now preserves untouched reference fields in returned
struct snapshots through scalar updates and nested extraction. It uses the
existing content model within the existing field and slot bounds. Replaced
fields, opaque writes, arrays, and exhausted bounds keep their distinct or
unknown evidence. The wrapper-chain bound remains four, and the analyzer adds
no copy traversal or logging contract. `returned_value_copies.go` pins the
returned handoff beside discarded and replaced-writer diagnostics.

For `DB.BeginTx` and `Conn.BeginTx`, cancellation of the exact acquisition
context triggers database/sql's rollback watcher. A direct or deferred call
of its paired cancel is therefore unknown cleanup, not synchronous rollback
or successful commit. A defer before acquisition must dominate it; later
actions use the ordinary obligation walk. This covers the [Odysee transactions](https://github.com/OdyseeTeam/odysee-api/blob/6cb1fd36ef7d25a038e3ddf572e3ddb3bbbb3d79/apps/watchman/olapdb/olapdb.go#L112-L139)
under the documented [BeginTx contract](https://pkg.go.dev/database/sql#DB.BeginTx).
The same structural context/cancel pairing serves pre-acquisition cancellation:
results zero and one from one standard `WithCancel`, `WithCancelCause`,
`WithDeadline`, `WithDeadlineCause`, `WithTimeout`, or `WithTimeoutCause` call.
Factory identity must agree; no alias traversal or deadline timing is inferred.
`transaction_cancellation.go` covers all six constructors, both receiver types,
prior and later actions, another context, replacement, conditional cancellation,
and `Begin` ignoring the context. The trace labels the action
`transaction-context-canceled` with outcome `unknown`. Cancellation can hide
an unintended rollback; this check proves resource loss, not transaction intent.

DB acquisitions through `PrepareContext`, `QueryContext`, and `BeginTx` are
known to fail when the exact context from one of those standard constructors
was synchronously canceled before the call. Conditional, deferred, or concurrent
cancellation does not establish this, nor do timing or test assertions.

A successful `errors.As` match on the exact acquisition error also establishes
failure. Matching an unrelated or joined error, or failing to match a type,
does not prove that the resource was never acquired.

`errors.Is` establishes the same failure on its true arm when the first
argument is the exact acquisition error and the target is a documented non-nil
filesystem sentinel, `context.Canceled`, or `context.DeadlineExceeded`. The
filesystem and context contracts share the sentinel lookup but retain distinct
trace reasons. An unrelated error, a joined error containing an independent
matching member, a possibly nil target, or a false match leaves acquisition
possible. Fixtures: `resourcelifetime/context_error_guards.go` and
`resourcelifetime/error_guards.go`. The context boundary is exercised by
[cute's timeout handling](https://github.com/ozontech/cute/blob/9f4583b9e8d9f5ac5771c15cc6a08c25d22ed2c3/roundtripper.go#L76-L91).

A visible boolean error helper can establish the same failed-acquisition
branch when every normal return for the exact nil error is literally false.
This includes callbacks passed through immutable lexical captures. All capture
origins must agree, and one shared work budget bounds the proof. Replaced or
escaped callback cells, rewritten errors, recursive predicates and deferred
result mutation do not establish that implication.

A called closure that guards an exact captured HTTP response's `Body` before
closing it is an ownership uncertainty boundary. The guard and `Close` may
read different loads, so this does not prove release. The response cell must
still hold the acquisition, with no visible pointer escape or replacement;
extra Boolean conditions and cleanup of another response do not qualify.

Channel timers and tickers are not cleanup obligations: since Go 1.23, the
garbage collector can reclaim them without `Stop`. Missing `Stop` alone does
not establish a leak. This check does not infer legacy main-module or
`GODEBUG=asynctimerchan=1` settings, nor claim that retained workers or
`AfterFunc` callbacks are harmless.

An exported constructor that returns a wrapper proven to hold the resource it
opened, such as a `*slog.Logger` over a log file, hands the resource to its
caller. The wrapper cannot release it, so the caller must keep the wrapper,
hand it on, or return it; a caller that drops it is reported with "resource
held by the result of ... is dropped on some return path". A wrapper that only
may hold its input, such as `bufio.NewWriter`, is not treated this way.

A resource that program exit reclaims is not reported. The acquisition must be
in `main.main` of package `main`, outside any loop or closure, in a package
that never calls its own `main`. It then runs at most once, and every way out
of `main` ends the process, which closes the file, response body, rows, or
statement. Cleanups with an effect that exit would lose are still reported:
compressors must flush, transactions must commit, and an inferred owner's
`Close` is not assumed to be effect-free.

## Contracts and transfers

Sending an aggregate that holds the exact acquired HTTP response Body is an
uncertain ownership handoff. A Body load must select that response's unmodified
field at the load point; current heap containment must then find that reference
in the aggregate before the send or select. This reuses projection stability
and point-in-time containment instead of walking historical stores. A saved
Body or aggregate copy may retain the original after later replacement.
Loading a replaced Body, sending an overwritten aggregate or another response's
Body, and passing response metadata or bytes do not establish this boundary.
`body_handoff_test.go` isolates these distinctions because broader rules can
already decline mutated responses. `body_handoffs.go` pins send/select forms
and keeps discarded aggregates, metadata and byte handoffs diagnostic.
The [ACP HTTP worker](https://github.com/Contextualist/acp/blob/579b477d0281df41ab8753a7cbcb8f7807e52e2c/pkg/pnet/p2p.go#L79-L91)
is the motivating value-copy shape. A send never proves cleanup; receiver
behavior and request cancellation are not assumed.


A resource stored through a loaded destination pointer is a handoff when the
heap model proves that every destination belongs to the same caller parameter.
A local struct or range table holding those addresses is not their owner.
A proven local destination keeps the obligation; replacement with a local
address therefore still reports. An unresolved loaded destination is opaque
consumption, never proof of cleanup. Mixed ownership, nil or opaque addresses,
and table windows or dynamic writes beyond the shared copy model keep unknown
ownership. This accepts coverage loss for unresolved local destinations rather
than asserting a leak through ambiguous storage. `indirect_destinations.go`
pins the accepted, reported, and opaque forms. The motivating
[ferro receiver storage](https://github.com/ferro-labs/ai-gateway/blob/d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4/internal/admin/repository/sql_store.go#L73-L99)
needs destination provenance, not a SQL-specific lifecycle exemption.


The `owned` contract family is not a table. A constructor in another package
whose returned struct holds a resource it acquired itself, and whose type has
a method that releases that field on every return, is summarized by the
lifecycle facts pass, and callers of that constructor then owe the method
exactly as they owe `Close` to `os.Open`. A wrapper that stores a caller's
resource, or a type whose methods never release the field, produces no
contract.

Two process and type boundaries are modelled rather than guessed. Files
listed in `os.ProcAttr.Files` and handed to `os.StartProcess` reach a call
with no visible body inside an aggregate, so the child's inheritance of the
descriptors is an opaque boundary and nothing is reported. A comma-ok
assertion of the resource to its own type, or to an interface it satisfies,
cannot fail for that value, so a release under the assertion is a release,
in the caller and inside a helper alike; an assertion the resource does not
satisfy leaves the release conditional.

A helper that releases every element of what it was handed inside a loop,
whether it ranges over a slice, an array, or a receiver field, is an
uncertainty boundary rather than a missing release: the completion search
reports that its only release lies inside a cycle, and the caller is not
reported. An imported helper carries the same loop as a may-claim in its
summary. A helper whose release depends on a flag has complete path
information and stays diagnostic, unless the call fixes the flag. A call
passing a constant Boolean, or a caller parameter that the caller's own
call fixed, binds the helper's parameter, and the completion search follows
only the branch that constant selects: `finish(file, false)` releases when
the helper closes under `!keep`, and `finish(file, true)` proves the leak.
The binding reaches a flag the helper tests inside a deferred closure through
the captured cell, when that cell is written once before capture and only
read after. An imported helper carries the same answer as argument cases in
its summary, bounded to two guarding Boolean parameters; a helper with more
keeps only its unconditional claims, so a constant call through it stays
diagnostic. A deferred call is bound too, so `defer finish(file, true)` is
reported rather than accepted as a release somewhere in the helper. A
variable flag, a flag merged from two branches, or a comparison of the flag
decides nothing. A nilable parameter the helper compares with nil is decided
the same way: `closeWithoutOptions(file, nil)` releases when the helper closes
under `options == nil`, and an allocated options value, a variable, or an
interface holding a typed nil pointer, which is not a nil interface, leaves
the file open. Fixtures: `resourcelifetime/argument_cases.go`.

A helper that acquires a resource and hands it straight back as a result,
such as `OpenConfig(path) (*os.File, error)` or one returning an
`io.ReadCloser`, is summarized the same way, and its callers owe the result
type's cleanup. That claim requires the value to reach the return untouched:
a helper that also registers it, calls a method on it, captures it in a
returned callback, or may have closed it before returning makes no claim,
so a dropped result from such a helper is not reported. Standard-library
packages the contract catalog already models keep the catalog's decisions;
an inferred owner never adds to them.

Fresh acquisition inference requires a known concrete resource type, in a
field or as the value returned. A custom `Close` method alone does not
establish acquisition: the result may be a lazy or empty handle. Nested
custom owners remain unknown, so leaks through those constructors may be
missed.

The same summaries decide the other direction. A call that stores the caller's
resource in its returned struct transfers the obligation only when that
struct's type has a method proven to release the field; a returned view such
as a buffered reader leaves the obligation with the caller, even when the
view's type has a `Close` of its own that releases nothing.

Every instruction after an acquisition is classified once as a release or
transfer, an opaque use, or nothing, and a leak is reported only when no
release or transfer covers a return and nothing opaque consumed the resource
on the way there. Summaries are the analyzer's knowledge: a summarized callee
that neither releases, stores, nor owns the resource is transparent, so a
read helper leaves the obligation in place. Opaque uses are real boundaries:
the resource handed to an interface method or function value, to a callee
with neither body nor summary, to a launched or deferred literal without a
proven release, or into a channel, map, or append the analyzer does not
track. Past such a use the analyzer stays silent rather than guess.

A deferred literal whose release turns on a named result is judged per
return, not at the defer. The close-on-error idiom,
`defer func() { if err != nil { f.Close() } }()`, runs after the return
statement has set `err`, so each return the defer dominates asks the shared
completion search whether the literal releases given the value that return
stores: a nil literal skips the cleanup and leaves the resource owned there, a
value never nil (a literal, an allocation, or a result the summaries prove
non-nil) runs it, and anything else makes that path unknown. A literal counts
as result-guarded only when its release on every return is proven under one
outcome of a captured named result and disproven under the other; a guard on
any other variable, such as the transaction idiom's `committed` flag, keeps
the data-dependent policy that credits a deferred literal which may release.
Fixtures: `resourcelifetime/result_guarded_defers.go`.

Literal outcomes precede the summary query. The unconditional guarantee's
`Outcome` projection is shared with cancellation ownership in `resultfacts`;
it neither spends another budget nor strengthens unknown result evidence.

A nil comparison is a presence check of the resource only when the compared
value can hold it: it derives from the resource and the resource's type is
assignable to it. An error returned by a helper that was handed the file
derives from the file, but no error value is the file, so `err != nil` after
such a call says nothing about whether the file exists.

A call can own more than one result. Each end of `os.Pipe` is its own
obligation, and a diagnostic names the end (`read end`, `write end`), since
closing one end releases nothing of the other. The pipe's error result guards
both. Fixtures: `resourcelifetime/pipes.go`.

An append into a local collection is tracked. When the resource is appended
to a slice the function made itself (nil, or `make`), and every use of every
version of that slice is understood, the resource stays owned through the
slice. The understood uses are further appends, the phis a loop merges them
in, `len` and `cap`, returning the slice whole (a transfer to the caller),
and reading elements only in a range loop that releases each one. That loop
must leave only through its length test, and the cleanup call on the element
must run on every iteration. It then settles the resource on its exit edge,
whatever the length. Any other use of the slice declines the model as a
whole and the append stays opaque: storing, passing, slicing, copying,
sending, or capturing it, a spread append, or reading an element elsewhere.
The decision is all or nothing because a loop that reads elements without
provably releasing them may still release this one, and the path that skips
it would otherwise read as a leak. A slice dropped still holding the
resource is reported. A deferred literal that drains a captured slice is
judged by the deferred-completion rule instead. Known gap: an acquisition
loop that returns on a later iteration's error does not report the resources
appended by earlier iterations, because the walk reads that error branch as
the current acquisition's own. Fixtures: `resourcelifetime/collections.go`;
the loop shape is `ssaflow.RangeElementLoop`.

Returning an indexed slice uses the ordinary shared returned-owner query,
independently of the append-collection model. Stores through the slice's own
element addresses are checked as well as stores through its backing owner.
This repairs the [sandbox file preparation](https://github.com/criyle/go-sandbox/blob/6a60e40be9d0cefb656c4ae12415c5fd040df954/cmd/runprog/fileutil.go#L6-L32):
success returns the populated slice, while a loop helper makes cleanup on the
error path unknown. It does not require an indexed-collection flow engine or
prove which element a cleanup loop releases. `returned_slice_elements.go`
retains diagnostics for another returned collection and a return that drops
the populated slice. Shared tests cover fixed and dynamic element stores and
unrelated values. Containment remains possible ownership, not exact release.

Passing the collection whole to a helper that releases every element of it on
every normal return is also understood, and settles the resource at the call,
as the loop's exit edge does. The helper's claim is the lifecycle discharge
at path `index:*` (see the fact model), or the same proof over the body of an
unexported helper in this package. A sub-slice or a copy is another value and
declines the model, and so does a helper that releases only some elements,
stops early, releases through a callback, keeps or appends to the slice, or
has no summary. Because declining is all or nothing, a helper that releases
every element on some returns but not others leaves the collection unknown
rather than reported; what is reported is a caller that skips the helper on
one of its own returns. Index loops, maps, and composite-literal roots are
not modelled yet. Fixtures: `resourcelifetime/collection_helpers.go`.

The same uncertainty applies when a retained aggregate argument contains the
resource, or a helper's aggregate result is published through a global. An
imported helper that receives the aggregate is judged by its summary's
kept-contents claim: one that keeps, sends, starts, returns, or hands to an
opaque callee anything loaded from the path where the resource sits, or that
has no summary at all, stays a boundary, while one proven to keep nothing at
that path leaves the obligation with the caller. A helper
that closes a merged resource argument or a body loaded from a merged response
also makes the result unknown: cleanup of the selected value does not prove which
acquisition it released. These boundaries do not establish ownership or cleanup.
Read-only helpers, overwritten response bodies, and cleanup of unrelated values
do not qualify for this merged-value boundary.

A deferred closure may close a captured variable assigned several acquisitions.
Its body must contain cleanup derived from that cell. A registration preceding
the acquisition must dominate it; a later registration is classified by the
ordinary path walk. Both use `capturedCellCleanup` to make the result unknown:
neither proves which value is closed, and either can miss overwritten-cell leaks.
Read-only captures, unrelated cleanup, and deferred arguments evaluated by value
do not establish this boundary. `deferred_reassigned_response.go` covers later
registrations, cleanup of another response, and a read-only deferred literal.
The [speedtest fallback request](https://github.com/anton48/vk-turn-proxy-ios/blob/001caf2ae24ecd07b021d7ca7b14a98a006bff65/third_party/speedtest-go/speedtest/server.go#L262-L285)
closes its first response before replacement; the defer closes whichever
response remains. Exact completion loses that path relation, so the shared
captured-cell uncertainty avoids a false leak claim without a second proof.

Compression writers over a local in-memory buffer are exempt, including
exact `bytes.NewBuffer` and `bytes.NewBufferString` results. Leaving one
unclosed holds nothing outside the function. Never closing it before the
buffer is read truncates the output, which is a data defect rather than a
leak and is not this check's claim.

A nil comparison of the resource settles the arm where it is nil, and the
same holds for the `Body` of a `net/http` response that is the resource: a
response whose `Body` is nil has nothing to close, and one returned without
error always has a body. A close guarded by `resp != nil && resp.Body != nil`
therefore covers every feasible path, and so does the negated `||` form that
returns first. Short-circuit operators are separate branches in SSA, so each
operand's edge is judged on its own. A guard computed into a variable first,
`ok := resp != nil && resp.Body != nil; if ok { … }`, branches on a phi of
Booleans. `ssaflow.BranchValueWithin` selects only the incoming operand belonging to
the flow state's predecessor, so the same presence proof applies to that exact
comparison. A missing predecessor, a phi from an earlier block, an unrelated
flag, or another response's body supplies no absence evidence. Fixtures:
`resourcelifetime/nil_guarded_bodies.go`.

A cleanup that reaches the resource through a generic helper's result, such
as `f := Must(os.Create(path))` closed from a deferred literal, is credited
because the points-to model applies the generic body to its instantiation
wrapper (see the points-to model note). A cleanup method value handed to a
helper that forwards it to a sibling which calls it on every return, such as
`defer decorate.LogFuncOnError(file.Close)`, is a release; a helper that may
return without calling it is not. Fixtures:
`resourcelifetime/generic_wrappers.go` and
`resourcelifetime/forwarded_callbacks.go`.

### Retired: use-after-release

A `use-after-release` check reported an operation documented to fail on a
released value, such as a write to a closed file, when a plain release on the
exact acquired value dominated the use in the same function. It was removed on
2026-09-27. Across the batch 56 to 61 audits, about 1,500 repositories, it made
three reports, all intentional negative tests that close a value and assert
the resulting error. The shape its proof required fails the first time the
code runs, so it rarely survives into a commit; real use-after-close bugs
cross goroutines, functions, or branch merges, which the proof deliberately
excluded. The `ReleasedUses` lifecycle fact that served only this check was
removed with it.

## Former public summary

Reports resources that are not released on every return path.

The built-in contracts cover files, transactions, SQL rows and statements,
HTTP response bodies, and gzip/zlib writers. A constructor in another package
that acquires one of these and returns it, or returns a struct with a method
that releases it, is inferred as an owner, so its callers owe the same cleanup.

A resource's obligation ends when it is released, returned, stored somewhere
that outlives the function, or handed to a callee proven to keep it. An
exported constructor that returns a wrapper holding the resource, such as a
`*slog.Logger` over a log file, hands the resource to its caller, which must
keep or pass on the wrapper. When the resource reaches code the analyzer
cannot see through, such as an interface method or a callee without a
summary, nothing is reported.

Some cases are deliberately not reported:

- channel timers and tickers, which the garbage collector reclaims since Go 1.23;
- compression writers over an in-memory buffer;
- a file, response body, or rows value acquired once in `main.main` of package
  `main`, which program exit closes. Compressors and transactions there are
  still reported, because exit would lose their flush or commit.
