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

Return disposition has one structured decision in `flow_returns.go`.
`proveResourceReturn` accepts the existing possible owner/alias handoff,
preserves an uncovered return as its diagnostic witness, and discards that
witness on child or parent cutoff. Its result census, wrapper-result census,
possible derivation and cleanup-method visits spend the flow allowance.
`heapmodel.ReturnedMayAliasAnyWithin` shares the result/candidate census and
alias dispatch; it preserves the original may-alias policy. Cutoff cannot
establish that no owner was returned. `flow_returns_test.go` covers direct,
aggregate, derived and known-owner handoffs against unrelated and scalar
results at every insufficient allowance. The heap census test uses two actual
SSA results and two candidates to check dispatch cutoff and fresh recovery.

The return census, derivation, recursive ownership, wrapper decoding and view
binding now share allowance. Graph, alias-query and type internals retain
independent costs; this is not a complete query bound.

The recursive lifecycle ownership entry owns alias evidence as well as
aggregate containment. Return results, loaded addresses, stored values and
constructor arguments delegate to it without first repeating `MayAlias`.
Its cycle guard records a pair only after negative alias evidence, while a
direct alias completes before that pair is recorded. Existing slice-owner,
captured-owner and delegated-constructor fixtures retain the possible-owner
policy and every-successful-return guarantee; containment still proves no
cleanup. Beads `gohawk-dho.44.11.5.18.1.1` records this consolidation.

Aggregate store enumeration now lives in `lifecycle.StoredIntoWithin` and
uses the shared keyed work driver rather than a recursive address visited set.
It follows only field/index addresses and pointer loads, yielding possible
stores with no ordering or observation-time guarantee. All existing consumers
ask an any-value containment question; none relies on store order. Address
visits and referrers spend the optional caller allowance, including revisits.
Consumer early stopping preserves availability; cutoff can leave a partial
sequence and cannot establish missing ownership. Actual SSA controls cover
fields, indexes, loaded callback slots, cyclic owners, early stopping and
callback pool exhaustion. The recursive returned-owner query uses its caller
allowance. Other consumers still use an unbounded allowance until their
classifiers propagate unknown. Beads `gohawk-dho.44.11.5.18.1.2` records the
shared traversal.

Resource returns now call `lifecycle.ProveReturnedOwnershipWithin` with the
flow allowance. The existing recursive ownership search shares result/value
visits, referrers, constructor arguments and instruction census, capture storage,
stored-value enumeration, access-path selections and constructor return coverage.
One structured proof discards both positive ownership and completed-negative
evidence at child or parent cutoff. Boolean defaults delegate to that proof.
Its negative outcome means no owner recognized by this model, not actual absence.

Constructor delegation lives in `store_constructors.go`: it requires an owner
on every successful return and retains the nil/error-only exception. That
exception's nilness query and the shared obligation walk now spend the same
allowance. Actual SSA controls cover direct and copied owners, helper stores,
conditional construction, uncovered sibling returns, closures, phis and cycles;
summary-hook pool cutoff cannot retain an owner, and a fresh query can retry.
Graph construction, alias-query internals, type-system work and external summary
hook internals still have independent costs. Broader classifier integration
remains under Beads `gohawk-dho.44.11.5.18.1`.

Wrapper return queries now charge explicit interface/type/boxing decoding,
constructor arguments, recursive chain steps and callee-claim dispatch against
that flow allowance. The four-step chain cap and must-hold constructor policy
are unchanged. `flow_wrapper_test.go` covers boxed direct/nested chains, unrelated
inputs and over-depth chains at every insufficient allowance.

Returned-view binding uses one domain proof through
`summaries.Provider.ProveCallReturnsViewWithin`. Selected declaration lookup,
argument visits and guarded containment share the allowance. Storage identity
keeps its independent `QueryBudget` cap inside the caller pool, and its cutoff
propagates as unknown even when the pool still has allowance. Ambiguous aliasing
cannot become exact binding through containment. Legacy Boolean binding callers
retain their previous independent cap and fallback; broader integration remains
open. `binding_budget_test.go` checks exact, unrelated, ambiguous and contained
arguments, fresh recovery, and an actual SSA conversion chain exceeding the
storage cap without exhausting its caller. Broker tests distinguish absent
knowledge from an available declaration with no view claim.

Return opacity now shares one completed wrapper proof with owner disposition,
including the direct result position. It uses the shared instruction census,
bounded dominance checks, wrapper-chain proof and recursive containment under
one caller allowance. A nested wrapper must be constructed before every path to
that return; a constructor confined to a sibling branch cannot cover it. A
positive possibility still labels opaque ownership, never settlement. Cutoff
labels `budget-exhausted`/unknown and is not memoized; completed negatives and
positives are reused. `returned_wrapper_test.go` covers direct/nested wrappers,
unrelated and discarded values, non-dominating construction, every insufficient
allowance, fresh classifier recovery and reuse without another search. The
obsolete Boolean/default facades are removed, and the return-specific chain
mechanics live with this proof in `flow_returns.go`.

Observed and timeless lifecycle containment now delegate to one structural
proof, with an explicit graph fallback chosen by their entry point.
`ProveMayContainValueAtWithin` preserves the existing observation policy,
including nil observations. The structural search may include later visible
stores; only the graph fallback is observed at the call. This remains possible
ownership and cannot establish exact before-call identity or cleanup. A graph
that cannot answer retains the legacy completed-negative/no-modeled-relation
outcome; budget cutoff remains unknown.

Aggregate escape classification now propagates argument-census and observed
containment cutoff through a structured proof. Direct resources and closures
retain their exclusions, retaining/effect evidence still decides publication,
and imported kept-contents facts retain their access-path policy. These rules
live with the other publication evidence in `ownership.go`. The lifecycle
observation controls cover boxing, direct and aggregate values, captures,
unrelated values, later visible stores and nil observations at every
insufficient allowance. The resource controls retain opaque/retaining helpers
against borrowing, unrelated values, direct resources and callbacks, plus fresh
classifier recovery after cutoff. Closure/wrapper/effect/path queries and
external-origin, graph/alias/type internals retain separate cost and availability
reviews under `gohawk-dho.44.11.5.18.1.5`.

Carried payloads now share `carried_values.go`'s structured direct/nested proof
with aggregate-escape argument binding. Sends, map updates, select sends and
call arguments propagate interrupted searches as unknown rather than a
negative carry result. Direct evidence combines possible alias/derivation with
exact local storage; storage keeps its independent query cap, whose exhaustion
remains unknown even when the caller has allowance left. Nested evidence keeps
object containment separate from the analyzer's derived stored-value fallback:
a pointer to a resource's field can be carried without containing the resource
object. Both families supply possible consumption, never cleanup.

`carried_values_test.go` exercises actual SSA for direct and loaded values,
aggregates, captures, unrelated inputs and derived field pointers, all
insufficient allowances, payload classifier recovery, and a conversion chain
exceeding the storage child cap. An overlay ignoring that child cutoff fails
the control despite the caller remaining available. Legacy closure/callee
consumers retain default adapters; possible-constructor execution, graph,
alias/type and effect internals still have separate costs. This closes only
`gohawk-dho.44.11.5.18.1.5.4.1`, not the parent classifier integration.

The carried-value local gate passed with tests at 74 seconds and repository
dogfood at 29 seconds. Immutable `.build/goal-carried-values-current`, SHA-256
`8a9bb5172a2d94ad79773a35459dcc61214ba88bdc579fd867b0bc8dcce4f85c`,
retains the pinned Cute/Ferro scopes above: exits 3/0, empty stderr, and
byte-identical JSON against the observed-containment binary. This records
stable scoped controls, not additional FP corrections or a full audit replay.

Possible constructor retention now has one structured engine in
`possible_wrappers.go`. Carried-value queries, observed aggregate argument
queries, strict retaining-call chains and foreign-owner stores share its
availability. The engine preserves the four-step depth cap, caller-selected
direct mode, scalar/error exclusions and nested derived-stored-value policy.
Missing call effects still permit possible retention; complete effects lacking
retention do not. Strict `ClaimStores` remains necessary to publish a chain
through a callee, and local allocations remain excluded from foreign stores.
This evidence never proves exact identity or cleanup. The separate returned
wrapper proof asks for must-hold result claims; combining the two policies
would strengthen possible retention into an unsupported guarantee.

Explicit append values use `ssaflow.AppendedValuesWithin`, which shares array
user/write census with the wrapper allowance and discards partial results at
cutoff. The default append helper delegates to the same mechanics. Spread
slices remain unsupported by this special constructor-chain traversal, while
ordinary argument containment still retains its existing policy.
`possible_wrappers_test.go` covers these distinctions, unknown on insufficient
allowance, publication through strict stores, local versus foreign destinations
and classifier recovery. The shared slice tests cover complete and interrupted
explicit/spread queries. Effect and graph/alias/type internals retain independent
costs: structural cutoff is unknown, while legacy effect-query uncertainty
continues to support possible retention. Closure/callee/default-query integration
remains tracked by `gohawk-dho.44.11.5.18.1.5.4`; this step owns child `.4.2`.
An overlay ignoring the wrapper allowance fails every wrapper cutoff control.

The possible-wrapper local gate passed. Immutable
`.build/goal-possible-wrapper-current`, SHA-256
`f90704431f5fcc5d78f6cf0be4be69e5753d5d91a59162752e9c62a030668643`,
retains the same Cute/Ferro pins and scopes: exits 3/0, empty stderr and
byte-identical JSON against the carried-value binary. These are scoped
stability controls; this step credits no new FP removal or corpus replay.

Closure and callback carrying now share `carried_callbacks.go`'s structured
proofs with aggregate argument exclusions, imported loop-release consumption,
and opaque closure calls. Capture matching delegates to
`heapmodel.CapturedBindingMatchesWithin`; broader capture contents delegate to
the ordinary carried-value proof. Argument/binding visits share the caller
allowance. Cutoff propagates as unknown instead of completing a negative
capture, retention or aggregate query. Positive evidence remains possible
consumption, never cleanup.

The callback recognizer still peels exactly one selected transparent step.
A twice-wrapped callback may carry a resource under broader value containment
without qualifying for this callback-specific policy. Direct resource arguments
and recognized resource-carrying callbacks remain excluded from struct-aggregate
ownership, while a separate aggregate argument can still carry the resource.
Known testing cleanup registration retains its separate coverage proof; a
visible callback observer and a summarized non-retaining helper stay transparent.
Imported loop-release claims still establish uncertainty, never completion.

`carried_callbacks_test.go` covers exact and nested captures, unrelated and
empty callbacks, one/two wrapper recognition, retaining/observing helpers,
mixed direct/aggregate arguments, callback exclusion, imported loop consumption
and started-closure classifier recovery at insufficient allowances. Prior
registration now uses the same bounded callback proof directly; the default
Boolean adapter and old carrying/aggregate/capture engines are removed. Effect, closure-retention, graph/alias/type, result-transfer
and aggregate-owner capture queries retain separate cost reviews; this step
does not claim an end-to-end query bound. Beads child `.18.1.5.4.3` tracks this
integration under `gohawk-dho.44.11.5`.
An overlay ignoring capture allowance fails all three binding-cutoff controls.

The closure/callback local gate passed (tests 77 seconds, repository dogfood
36 seconds). Immutable `.build/goal-carried-callbacks-current`, SHA-256
`b605676e36cfd22dbeb2869ea433b06e9b88f7dafa55523bfe853824529a68d7`,
retains the same pinned Cute/Ferro scopes: exits 3/0, empty stderr and
byte-identical JSON against the possible-wrapper binary. This credits stable
scoped controls, no new FP removal and no full precision replay.

Call-result publication now has one instruction census in
`ownership.go:proveCallResultMayTransferWithin`, shared by return and global
store witnesses. Result visits and may-derivation share the publication
allowance. A completed witness makes a nested argument's ownership uncertain;
an interrupted search likewise suppresses the leak, with a budget reason,
rather than completing a false publication result. Neither establishes cleanup.
The classifier consumes this structured proof after proving a nested argument,
so it preserves one authoritative publication decision.

The existing filters remain distinct: returned errors are excluded, while
global stores exclude both errors and scalar observations. Returned scalars
retain the existing broader may-derivation policy. Foreign field stores and
discarded/unrelated results remain outside this result-publication query.
`result_publication_test.go` covers these shapes, all insufficient allowances,
fresh recovery, and a real SSA census exceeding the child cap while the
candidate pool remains available. An overlay ignoring publication allowance
must fail both the census and classifier controls. The shared proof allowance
test harness is separate from callback/publication scenario fixtures. Graph,
alias/type and effect internals retain independent cost reviews; this step owns
`gohawk-dho.44.11.5.18.1.5.5`, not the broader integration goal.

The result-publication local gate passed (tests 61 seconds, repository dogfood
23 seconds). Immutable `.build/goal-result-publication-current`, SHA-256
`a07afd450724e6c3db2726a3a9ae14356daea790fd0fa9b8dcc6d04dacc4c9b3`,
retains the same pinned Cute/Ferro scopes: exits 3/0, empty stderr and
byte-identical JSON against the closure/callback binary. This records scoped
stability, no new FP removal and no full precision replay.

Asynchronous exposure now uses `ownership.go:proveAsynchronousExposureWithin`.
Argument visits, derivation and imported async-claim dispatch share the caller
allowance. The local fallback requests `LifecycleEvidence.CallEffectsWithin`,
which retains its existing query cap while charging the caller. An interrupted
effect query is unknown even when the resource's caller allowance remains
available; it cannot complete a negative exposure result. Positive imported
async claims and complete local async effects retain the existing opacity
reason. Complete read-only helpers, unrelated arguments and unavailable bodies
retain their earlier exposure-policy result; other classifier stages still
decide opaque calls. No exposure result proves cleanup.

The broker preserves its default cap and trace when passed a nil allowance.
All resource effect consumers now pass their allowance through `CallEffectsWithin`;
the unused default-only broker method has been removed. This does not claim a
transitive bound over graph/alias/type or call-binding construction internals.
`call_effects_budget_test.go` checks exact read/async/unavailable effects, every
insufficient allowance and the local child cap. `asynchronous_exposure_test.go`
covers imported and local exposure, accepted borrowing/unrelated forms, and
unknown at the classifier when the effect child exhausts while the candidate
pool remains available. An overlay ignoring effect-cutoff propagation fails
that consumer control. This step owns `gohawk-dho.44.11.5.18.1.5.6`; captured
owner and access-path availability remain parent work.

The async-exposure local gate passed; its immutable executable is
`.build/goal-async-exposure-current`, SHA-256
`d27280a620970129ca0cc05e5f1967b426bd5accb107e5449d838691c7877d03`.
The same pinned Cute/Ferro scopes exit 3/0 with empty stderr and byte-identical
JSON against the result-publication binary. Ignoring effect-cutoff propagation
fails both the independent proof and authoritative classifier controls. These
receipts establish scoped stability, with no production FP removal credited
and no full precision replay.

Captured aggregate-owner matching now has one structured decision in
`captured_cleanup.go:proveCapturedAggregateOwnerWithin`. Owner visits and alias
dispatch share the caller allowance, and capture matching delegates to
`CapturedBindingMatchesWithin`. The closure classifier preserves interrupted
searches as unknown; possible owner identity still supplies uncertainty rather
than cleanup. The pointer-struct restriction and resource-self exclusion are
unchanged. Owner discovery and graph/alias/type internals retain independent
costs; this does not bound them transitively.

`captured_owner_test.go` checks captured and reassigned owners, unrelated
captures, scalar/value owners, the resource-self exclusion and no owners.
The late-populated local-owner control also consumes actual owner discovery:
SSA exposes both the captured pointer cell and the holder loaded from it,
and the proof keeps the existing type exclusions at that boundary.
It checks insufficient allowances, child cutoff with an available parent,
classifier cutoff and fresh recovery. An overlay removing the shared allowance
must fail the proof and classifier controls. Beads
`gohawk-dho.44.11.5.18.1.5.7` owns this boundary; observed access-path availability
remains parent work.

The captured-owner local gate passed; immutable
`.build/goal-captured-owner-current`, SHA-256
`959891bd1bcd274046585f5ed024bed13ef297170874047b21ea6396bc3d1d2d`,
retains byte-identical JSON against the async-exposure binary in the same pinned
Cute/Ferro scopes, with exits 3/0 and empty stderr. The unbounded overlay fails
the allowance, child-cutoff and captured-owner classifier controls. No full
precision replay or production FP removal is credited.

Observed aggregate paths now consume `resourcemodel.ProveRelation` directly
under a capped child of the aggregate-escape allowance; the string-only
`pathWithin` adapter is removed. The shared relation stops after interrupted
identity and delegates stored paths to `heapmodel.ProveStoredPathWithin`.
Budget cutoff returns resource unknown before querying kept contents, including
a local child cutoff with an available caller. A completed unavailable position
retains the existing whole-aggregate query. Path identity identifies the slot;
it does not prove ownership or cleanup, and fact encoding is unchanged.

`aggregate_path_test.go` checks imported retention of the exact field versus
an independent sibling field, interrupted allowances and classifier recovery.
The shared tests cover overwritten contents, the two-selection boundary and
structural storage cutoff with parent allowance left. Conditional result-set
release remains covered by its existing exact-field control. Beads
`gohawk-dho.44.11.5.18.1.5.8` owns this integration; graph/alias/type internals,
owner discovery and other independently bounded effects remain separate work.

The observed-path local gate passed. Immutable
`.build/goal-observed-path-current`, SHA-256
`45fc67656ca0a3d3d8cafc4876c2a9f45b41c70dd68ba7119b9947ff2c8e852e`,
keeps the same pinned Cute/Ferro scopes byte-identical to the captured-owner
binary, with exits 3/0 and empty stderr. Restoring unbounded identity-to-path
fallback fails both shared relation controls; ignoring the resource contents
allowance fails the exact-field and sibling-field controls. No full precision
replay or production FP removal is credited. The two remaining default effect
consumers are tracked together by child `.18.1.5.9`.

Those two consumers now request `LifecycleEvidence.CallEffectsWithin` with their
current allowance: `ownership.go:proveAggregateOwnerEscapeWithin` and
`possible_wrappers.go:provePossibleWrapperWithin`. The effect query retains
its local cap and authoritative trace path. A shortened query returns resource
unknown before kept-content fallback or possible wrapper retention, even when
the caller still has allowance. Complete read/retention/async results retain
each consumer's existing policy; unavailable bodies retain their prior fallback.
Wrapper retention still requires its existing retain effect, rather than
treating every possible asynchronous use as ownership of the returned wrapper.

`ownership_effects_test.go` covers visible borrowing, retention and asynchronous
exposure, unrelated values and unavailable bodies, then long read-only helpers
whose effect child exhausts while the caller remains available. It checks both
the aggregate and wrapper proofs and the aggregate-call/foreign-store
classifiers, alongside short-helper recovery. Ignoring cutoff propagation must
fail the independent proof and classifier controls. These visits share the
candidate allowance; graph/alias/type/binding internals retain independent costs.

Migrating the final consumers made the default-only `LifecycleEvidence` method
unreachable. It is removed rather than retained solely for tests; the lower
allowance and cap controls now compare against `CallEffectsWithin(..., nil)`.

The final ownership-effect gate passed after that removal. Immutable
`.build/goal-ownership-effects-final-current`, SHA-256
`200e719c1382a4f78fc20eb47f688a68edad73752e54d5db4520985f13f35945`,
retains byte-identical JSON against the observed-path binary in the same pinned
Cute/Ferro scopes, with exits 3/0 and empty stderr. The ignored-cutoff overlay
fails all three independent proof controls and both classifier controls.
The nine-child ownership-integration source review is recorded in the
consolidation completion audit. Pre-acquisition work remains `.44.11.5.17` and
cleanup/context uncertainty remains `.44.11.5.19`; no production FP removal or
full precision replay is credited.

The resource projection decision propagates view-binding and storage-projection
cutoff rather than treating it as missing evidence. A known non-cleaning view
still cannot discharge its resource merely because it has a Close method;
missing declarations retain the existing method-set fallback. Graph construction,
graph-query internals, alias/type queries and fact-copy costs remain separate.
These changes do not establish cleanup through possible containment.

The binding-cutoff overlay deliberately treats an interrupted view query as
missing evidence; the resource control then fails because method-set acceptance
wins despite the caller pool remaining available. This pins cutoff propagation
at the consumer as well as the domain proof. Scoped resource controls use
immutable `.build/goal-view-binding-current`, SHA-256
`6e31425bec28db667942e9594c16b018a189256d95cabd9157e158cb36f7cf63`.
Cute and Ferro retain the pins/scopes above, exit 3 and 0 respectively, and
byte-identical JSON versus the recursive-owner binary. Both stderr files are
empty. This credits no additional FP correction or corpus result.

The scoped resource controls above were repeated with immutable
`.build/goal-return-owner-complete`, SHA-256
`bb25d0ed5e66acc1cca3184f761876caf2ca591177f3a90b7b5f799ea54b9e39`.
Cute again completes with exit 3 and its 980-byte reviewed-TP JSON; Ferro
completes with exit 0 and `{}`. Pins, scopes and flags are unchanged, both
stderr files are empty, and both outputs are identical to the presence-proof
binary. The unbounded alias-census overlay fails the zero-allowance control.

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
owner. It reuses `ssaflow.AppendedValuesWithin`; spread slices remain outside this
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

Store disposition now returns the destination owner beside one structured proof.
Owner collection no longer repeats stored-value derivation and containment to
classify the same destination. During resource classification, release and
opaque-consumption decisions reuse the completed disposition for that exact
store. Cutoff answers are never memoized. Stored-value derivation and recursive
containment share a `SummaryBudget` query drawn from the candidate pool; cutoff
is unknown and cannot settle transfer. Owner discovery now initializes the same
observed candidate pool before its census and spends the existing 250,000-step
release-query allowance on instruction visits, storage disposition and owner
alias deduplication. A completed census atomically seeds the existing owner list
and store-disposition cache; interrupted collection discards both and stops
before flow classification. Possible holders still establish no cleanup or
ownership guarantee. Local, foreign, unrelated and ambiguous destinations keep
their prior policy. Destination-origin and graph/alias/type internals retain
independent costs. Earlier acquisition predicates remain separate work under
`gohawk-dho.44.11.5.17`. `storage_test.go` checks actual SSA for foreign/local,
unrelated/contained, copied/replaced and opaque destinations at every
insufficient allowance, plus fresh retry and reuse without a second search.
`owner_discovery_test.go` checks the candidate census at every insufficient
allowance, child cutoff with parent allowance remaining, atomic commitment on a
fresh query, classifier-cache reuse with no available pool, and complete-flow
leak/release controls. Small actual-SSA inputs exercise the same discovery
boundary without generating a fixture at the production quota.

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
ordinary path walk. Both use `proveCapturedCellCleanupWithin` to make the result unknown:
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

### Prior cleanup registration allowance

The prior-registration query now returns one structured proof with its witness
instruction. Deferred captured-cell cleanup and known testing cleanup callback
registration retain their may-cleanup/opaque-consumption polarity; neither
settles the resource. Both instruction censuses, capture/binding visits,
callback argument recognition and captured-body access-path matching share the
candidate pool. A child cutoff returns budget uncertainty before resource flow.
The same captured-cell proof supplies the ordinary deferred-closure classifier,
so that path cannot treat interrupted evidence as transparent consumption.
Tracing uses the returned witness rather than repeating the cleanup decision.

The callback recognizer retains its single transparent wrapper step. Database
statement-parent, rows-transaction and paired transaction-context contracts keep
their existing independent identity queries; graph/alias/type internals and
earlier acquisition predicates remain separate cost work. Deferred witnesses
keep precedence over callback registrations. `prior_cleanup_test.go` covers
mutable captured cleanup, exact testing registration, unrelated captures,
by-value defers, later registration, all insufficient allowances, child/pool
cutoff, fresh recovery and full-flow leak/release controls using actual SSA.
