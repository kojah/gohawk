# Shared helpers

The `syntax`, `ssaflow`, and `lifecyclefacts` helpers, indexed by the
question each one answers, followed by the generated index of every exported
helper.

Every analyzer stands on the shared engine in `internal/ssaflow` and
`internal/passes/lifecyclefacts`. Before writing any traversal, provenance, or
ownership code, find the question below. The authoritative source is the doc
comment on each helper (`go doc ./internal/ssaflow`); this page indexes them by
purpose so the right one is easy to find.

Shared code provides mechanics — how to walk — and never decides whether
evidence is sufficient for a diagnostic. That policy stays beside each
analyzer.

## What flows into this value?

Value-provenance folds own the recursion, the visited set, and the fan-out
over phi merges. Analyzers supply only a leaf predicate and the transparent
forms to look through. `TestAnalyzersUseSharedTraversal` rejects an analyzer
that fans out over phi edges or threads its own visited set.

| helper | answers |
|---|---|
| `NewReachingWalk(forms)` with `Any`, `Every`, and `EveryOf` | does some / every value reaching here satisfy the predicate? |
| `ResolveReachingValue` | do all paths agree on one leaf, or is it ambiguous? |
| `ValueDerivesFrom`, `MayAliasThroughLoads` | is this value a wrapped, loaded, or merged form of that one? (possible identity) |
| `DefinitelySameValue` | definite value identity; does not infer equality from mutable storage history |
| `MayAlias`, `MayAliasAny`, `MayContainValue` | possible identity or containment; the May prefix is the contract: never a guarantee that an action settles the target |
| `ProveIdentity`, `SameAccessPath`, `ValueIsAccessPathFrom` | structured identity of two access paths; is this value a field or index path from that root? |
| `Storage.Resolve`, `Storage.Content`, `Storage.Same` | what value does local storage contain at this point, and does its identity agree? |
| `Storage.StableContent`, `Storage.Projection` | is captured storage stable, or is an acquired owner's projection still unmodified? |
| `IdentitySource` | the identity-only source behind wrappers and loads (not for ownership) |
| `PhiIncoming`, `PhiEdgeCount` | inspect one phi without walking it yourself |
| `UnwrapTransparentValue(value, forms)` | peel exactly the wrapper forms the caller selects |

There is deliberately no universal unwrap helper. Callers select the exact
transparent forms that are sound for their proof and keep a fixture for a
wrapper that must stay opaque.

## Does an action cover every return?

The flow query behind classify-then-flow. `EvaluateObligation` is the one
walk: the analyzer labels instructions, returns, and edges as exact, unknown,
or none, and the walk reports the weakest coverage found on any feasible path
to a normal return. Uncertainty stays on the path where it occurs. The Boolean
`UnownedReturn` family is the same walk on a two-level lattice for callers
that only need to know whether some return is uncovered.

| helper | answers |
|---|---|
| `EvaluateObligation(ObligationFlow)` | honored when exact actions cover every return, uncertain when some return is reached only through an opaque action, violated when a return has no action at all |
| `UnownedReturn`, `UnownedReturnFromEntryWithEdges`, `UnownedReturnFromEntryAllow` | is there a reachable normal return with no owning action before it? |
| `UnownedReturnAssumingNonNil`, `UnownedReturnFromEntryAssumingNonNil` | the same, on paths feasible when a value is non-nil |
| `UnownedReturnAfterCallSuccess` | the same, after a call's success branch |
| `NormalReturnReachableFrom` | can a normal return be reached from here? |
| `FeasibleSuccessors`, `SuccessBranch`, `BlockReachable`, `BlockInCycle` | control-flow feasibility primitives |
| `InstructionDominates`, `InstructionMayFollow`, `InstructionIndex` | ordering between instructions |

## Carry a state along every path

| helper | answers |
|---|---|
| `WalkStates(initial, key, step)` | a keyed work list over path-sensitive states; the caller owns the state type and transfer, the driver owns termination |
| `InstructionsReachableAfter(start)` | every instruction reachable forward from a point |
| `InstructionsOf[T](function)` | every instruction of one type in a function |

## Did the callee finish the obligation?

| helper | answers |
|---|---|
| `MethodCallCoverage`, `ValueCallsMethod` | does the launched callee call the lifecycle method before each normal return? |
| `ProveCompletion` | the structured completion proof behind those |
| `CallInvokesArgumentOnEveryReturn`, `SpawnInvokesArgumentOnEveryReturn`, `DeferredClosureInvokesArgumentOnEveryReturn` | is the func argument itself invoked synchronously on every return, including inside a spawned wrapper? |
| `DeferredClosureCallsValue`, `ClosureCallsValue`, `CallReturnsDeferredCleanup` | deferred and callback cleanup shapes |

## Could a helper hide evidence that changes this check's answer?

Prefer shared infrastructure whenever a check needs interprocedural reasoning,
but introduce new summary families selectively:

1. Start from a concrete missed case or duplicated helper traversal, not a
   mandate to summarize every check. Local syntax and API-call checks usually
   need no interprocedural model.
2. Reuse `LifecycleEvidence`, completion proofs, and `CallEffects` first. For
   genuinely new context-independent evidence, `FunctionSummaries` shares
   memoization, recursion handling, work budgets, and call-site binding.
   Use its underlying `CallGraphMemo.Summarize` driver for context-sensitive
   questions, retaining target, mode, and binding keys; function identity alone
   is not an adequate cache key for them. Use `CallGraphMemo.Compose` with
   `CallGraphMemo.WithFunction` when one cached question guards several bodies.
   Raw cache and guard operations belong only to the summary implementation.
3. State each summary's guarantees and uncertainty. Distinguish a complete
   empty effect sequence from an unavailable sequence, and a set of positive
   witnesses from an exhaustive effect set. No recorded effect is not proof
   that the function has no effect. Never use a shortened summary to establish
   absence, and charge nested work to the same budget.

Lock operations, goroutine completion and joins, channel operations, cleanup,
cancellation, and ownership transfer are candidates for this reuse. Summaries
do not remove the need to prove aliasing, path conditions, or synchronization;
they do not by themselves solve arbitrary concurrent execution.

## Where did the value go?

Storage, escape, and transfer checks used by the classifiers to tell a
transfer from an opaque escape.

| helper | answers |
|---|---|
| `StoresValueInField`, `StoresValueInGlobal`, `StoresValueInEnclosingScope`, `StoresValueInEscapingField`, `StoresValueInOwnedMap` | where a store puts the value |
| `StoresOwnerOfValueInField`, `StoresOwnerOfValueInExternalField` | a store of the value's owner |
| `SendsValue`, `ClosureCapturesValue`, `ValueEscapes`, `ExternallyOwnedValue` | sends, captures, and escapes |
| `CallTransfersValueToField`, `CallTransfersArgumentToReturnedOwner`, `CallTransfersArgumentToReceiver`, `CallTransfersArgumentToLifecycleOwner` | ownership transfer through a call |
| `ReturnedValueOwnsValue`, `ReturnedMayAliasAny` | does a return carry the value or its owner? |
| `ClosureBindingPairs` | the captured variables of a closure paired with the values supplied for them |
| `CapturedBindingValue`, `CapturedBindingMatches` | inspect one captured binding |

## Which call is this?

| helper | answers |
|---|---|
| `CallEffects.Value`, `CallEffects.Call`, `CallEffectProof.PreservesStorage` | bounded local read/mutate/retain/async/invoke evidence, without ownership policy |
| `CallBindings`, `DirectCallee` | statically known callee and argument/capture mapping, without alias or completion claims |
| `CallMatchesSymbol`, `CallMatchesAnySymbol`, `ValueMatchesSymbol`, `ValueMatchesAnySymbol` | does this call or value resolve to an exact well-known declaration? |
| `HasLibraryContract` | does this call match a registered external API contract? |
| `InstructionCall`, `CallName`, `CallReceiver`, `CallResult` | call metadata |
| `SourceSSAFunctions`, `FunctionFile` | source functions and their files |
| `InstructionTerminatesControlFlow`, `SpawnedValueAtCall`, `ChannelType`, `DefinitelyNil` | miscellaneous facts about instructions and values |

Match well-known functions through `syntax.Symbol`; do not reconstruct
identity from package paths and raw names.

## Match source and symbols

`internal/syntax` is the source-level layer: well-known symbol identity for
both AST and SSA matchers, and the AST helpers analyzers that work on syntax
share. Name-only matching is reserved for documented external contracts.

| helper | answers |
|---|---|
| `PackageFunction`, `PackageMethod`, `PackageVariable`, `Builtin` | build a `Symbol` for an exact declaration; the SSA matchers above and the AST matchers here take these |
| `IsCallTo`, `IsCallToAny` | does this call expression resolve to the symbol, by type information rather than name? |
| `NamedType`, `IsErrorType` | named type identity and error interface implementation |
| `SameExpression`, `Unparen`, `ExpressionUsesObject` | structural equality of expressions, parenthesis stripping, and whether an expression reads an object |
| `FunctionParameterObject` | the object a parameter identifier declares |
| `GeneratedFile`, `SourceRange`, `AnalyzeFile`, `ShortPackageName` | skip generated files, recover a source range from a node, decide whether a file is analyzed, and abbreviate a package path for messages |

## Cross-package lifecycle facts

| helper | answers |
|---|---|
| `lifecyclefacts.NewLifecycleEvidence` and `LifecycleEvidence.Prove` | the one decision path for consulting local evidence and then imported facts |
| `lifecyclefacts.ResourceCleanup(type)` | which methods release a resource of this type |
| `lifecyclefacts.CallReturnsView` | does the call return a view onto its argument rather than a new owner? |

Consumers go through `LifecycleEvidence`. Raw `analysis.Pass.ImportObjectFact`
and `analysis.Pass.ExportObjectFact` calls belong only in the package that
defines the fact type; `TestObjectFactsStayInTheirDefiningPackage` enforces it. See
[Inferred facts](../../../../docs/development/fact-model.md) for what a fact can express.

## Adding a helper

Promote a mechanic here only after a second analyzer needs it. Make it
higher-order — it takes the leaf predicate — and form-selective — it takes the
transparent forms — so the next consumer reuses it instead of copying it.
Keep the doc comment as the contract: the index below regenerates from it,
and the architecture tests fail if a helper is missing from it. Then add the
helper under the question it answers above.

## Index of exported helpers

Every exported function and method of `internal/syntax`, `internal/ssaflow`,
and `internal/passes/lifecyclefacts`, with the synopsis of its doc comment.
Regenerated by `go generate ./...`; do not edit by hand.
`TestDocumentationReferencesResolve` fails when a helper is missing from it.

<!-- gohawk:generated-helpers:start -->
| helper | package | what it does |
|---|---|---|
| `AnalyzeFile` | syntax | AnalyzeFile reports whether file is the canonical copy to analyze. |
| `BlockInCycle` | ssaflow | BlockInCycle reports whether control flow can return to start. |
| `BlockReachable` | ssaflow | BlockReachable reports whether target is reachable from within their shared function. |
| `Builtin` | syntax | Builtin identifies a predeclared Go function. |
| `CallBindings` | ssaflow | CallBindings maps arguments and captures onto a known callee. |
| `CallEffectProof.PreservesStorage` | ssaflow | PreservesStorage proves no write, retention, asynchronous exposure, or callback invocation through this address. |
| `CallEffects.Call` | ssaflow | Call summarizes a call's uses of exactly value, through every matching argument and capture. |
| `CallEffects.Value` | ssaflow | Value summarizes all visible uses of a parameter or captured address. |
| `CallGraphMemo.Answer` | ssaflow | Answer returns the memoized answer for key, computing it once. |
| `CallGraphMemo.Compose` | ssaflow | Compose memoizes a context-keyed question that may inspect multiple function bodies. |
| `CallGraphMemo.Cut` | ssaflow | Cut records that an answer was shortened for a reason of the caller's own, such as an exhausted budget, so that answer is not retained either. |
| `CallGraphMemo.Enter` | ssaflow | Enter marks function as being on the current path. |
| `CallGraphMemo.Incomplete` | ssaflow | Incomplete reports a policy-specific truncation, such as a revisited callback value outside the function recursion guard. |
| `CallGraphMemo.Leave` | ssaflow | Leave un-marks function as the walk returns past it. |
| `CallGraphMemo.Summarize` | ssaflow | Summarize composes one context-keyed question about a function body. |
| `CallGraphMemo.WithFunction` | ssaflow | WithFunction executes visit inside one callee's recursion scope and always releases that scope on return. |
| `CallInvokesArgumentOnEveryReturn` | ssaflow |  |
| `CallMatchesAnySymbol` | ssaflow | CallMatchesAnySymbol reports whether common statically resolves to one of symbols. |
| `CallMatchesSymbol` | ssaflow | CallMatchesSymbol reports whether common statically resolves to symbol. |
| `CallName` | ssaflow | CallName returns a statically known method, function, or builtin name. |
| `CallReceiver` | ssaflow | CallReceiver returns receiver value for method calls and invocations. |
| `CallResult` | ssaflow | CallResult returns the selected SSA result of call. |
| `CallReturnsDeferredCleanup` | ssaflow |  |
| `CallReturnsView` | lifecyclefacts | CallReturnsView is ArgumentReturnedAsView for callers that hold the pass rather than an evidence context. |
| `CallTransfersArgumentToLifecycleOwner` | ssaflow | CallTransfersArgumentToLifecycleOwner recognizes a consumed value only when the call returns an escaping object with an explicit cleanup lifecycle. |
| `CallTransfersArgumentToReceiver` | ssaflow | CallTransfersArgumentToReceiver reports whether a source-visible method stores an argument in a receiver that outlives the call. |
| `CallTransfersArgumentToReturnedOwner` | ssaflow | CallTransfersArgumentToReturnedOwner reports whether a source-visible callee hands the argument back inside every value it returns, and the caller then lets that result escape. |
| `CallTransfersValueToField` | ssaflow | CallTransfersValueToField reports whether a call consumes value and stores its result in a struct field, transferring cleanup to the receiving owner. |
| `CapturedBindingMatches` | ssaflow | CapturedBindingMatches reports whether a closure binding directly contains target or refers to an addressable local that has contained target. |
| `CapturedBindingValue` | ssaflow |  |
| `ChannelType` | ssaflow | ChannelType reports whether value has channel type. |
| `CleanupFact.AFact` | lifecyclefacts | AFact marks CleanupFact as an analysis fact. |
| `CleanupFact.DescribeFact` | lifecyclefacts | DescribeFact renders the contract for the fact dump, naming the fields the methods release so a reader can check the claim against the struct. |
| `CleanupFact.String` | lifecyclefacts |  |
| `ClosureBindingPairs` | ssaflow |  |
| `ClosureCallsValue` | ssaflow | ClosureCallsValue reports whether a call-like closure or created callback calls target. |
| `ClosureCapturesValue` | ssaflow | ClosureCapturesValue reports whether instruction creates a closure that owns value. |
| `DeferredClosureCallsValue` | ssaflow | DeferredClosureCallsValue reports whether a deferred closure calls target. |
| `DeferredClosureInvokesArgumentOnEveryReturn` | ssaflow | DeferredClosureInvokesArgumentOnEveryReturn reports whether a deferred closure delegates target to a helper that invokes it on every normal path. |
| `DefinitelyNil` | ssaflow | DefinitelyNil reports whether every represented SSA value is nil. |
| `DefinitelySameValue` | ssaflow | DefinitelySameValue proves value identity without possible-alias or storage history matching. |
| `DirectCallee` | ssaflow | DirectCallee returns only the statically named function or literal body. |
| `ElementOfAggregate` | ssaflow | ElementOfAggregate reports whether value was selected from an element of a slice, array, map, or range iteration, directly or through fields and loads. |
| `EmbeddedFieldPath.Append` | ssaflow | Append extends a path without changing its root, or declines an invalid or over-budget path. |
| `EvaluateObligation` | ssaflow | EvaluateObligation carries the classifier's labels along every feasible path after the obligation and returns the weakest return coverage found. |
| `ExpressionUsesObject` | syntax | ExpressionUsesObject reports whether node refers to object. |
| `ExternallyOwnedValue` | ssaflow | ExternallyOwnedValue reports whether value comes from storage that outlives the current function invocation. |
| `Fact.AFact` | lifecyclefacts |  |
| `Fact.Claim` | lifecyclefacts | Claim returns the parameters this summary makes the claim about. |
| `Fact.DescribeFact` | lifecyclefacts | DescribeFact renders the summary for the fact dump: one line per parameter that some mask covers, named from the function's signature. |
| `Fact.MethodMask` | lifecyclefacts | MethodMask selects the parameter mask for a lifecycle method. |
| `Fact.String` | lifecyclefacts | String decodes the masks by parameter position so the fact is readable in analysis debug output. |
| `FeasibleSuccessors` | ssaflow | FeasibleSuccessors preserves constants selected by predecessor-sensitive phis and literal results of bounded, source-visible helpers. |
| `FunctionFile` | ssaflow | FunctionFile returns source file containing function. |
| `FunctionParameterObject` | syntax | FunctionParameterObject returns the declared object at the positional parameter index. |
| `FunctionSummaries.AtCall` | ssaflow | AtCall resolves a direct function or closure, obtains its symbolic summary, and supplies parameter/capture bindings to instantiate it. |
| `FunctionSummaries.Function` | ssaflow | Function returns the symbolic summary of function. |
| `GeneratedFile` | syntax | GeneratedFile reports whether file carries Go's generated-file marker. |
| `HasLibraryContract` | ssaflow | HasLibraryContract reports whether common exactly matches a registered API. |
| `IdentitySource` | ssaflow | IdentitySource returns the operand a value is an alias of for identity resolution: every transparent wrapper, and a load, because `*p` names the same lock or context as the cell p holds. |
| `InstructionCall` | ssaflow | InstructionCall returns call metadata carried by call-like SSA instructions. |
| `InstructionDominates` | ssaflow | InstructionDominates reports whether every path to after executes before. |
| `InstructionIndex` | ssaflow | InstructionIndex returns instruction position within its basic block. |
| `InstructionMayFollow` | ssaflow | InstructionMayFollow reports whether after is reachable after before. |
| `InstructionTerminatesControlFlow` | ssaflow | InstructionTerminatesControlFlow reports calls whose documented behavior prevents execution from continuing in the current goroutine. |
| `InstructionsOf` | ssaflow |  |
| `InstructionsReachableAfter` | ssaflow | InstructionsReachableAfter returns every instruction that control can reach after start without crossing a loop back edge, in visiting order. |
| `IsCallToAny` | syntax | IsCallToAny reports whether call statically resolves to one of symbols. |
| `IsCallTo` | syntax | IsCallTo reports whether call statically resolves to symbol. |
| `IsErrorType` | syntax | IsErrorType reports whether value implements Go's predeclared error interface. |
| `LifecycleEvidence.ArgumentRetainedByCallee` | lifecyclefacts | ArgumentRetainedByCallee reports whether the call's static callee is summarized as keeping the argument that contains target somewhere other than its returned value: a logger sink, a registry, a receiver field. |
| `LifecycleEvidence.ArgumentRetained` | lifecyclefacts | ArgumentRetained reports whether the summary of the call's static callee marks the argument at index as retained. |
| `LifecycleEvidence.ArgumentReturnedAsView` | lifecyclefacts | ArgumentReturnedAsView reports whether the call's static callee is summarized as returning a view over the argument that contains target: the argument is stored in the returned struct and nothing on that type releases it. |
| `LifecycleEvidence.CallEffects` | lifecyclefacts | CallEffects exposes local call-effect evidence beside lifecycle evidence, without treating an absent Retained bit as a read-only contract. |
| `LifecycleEvidence.CalleeClaims` | lifecyclefacts | CalleeClaims reports what the call's static callee is summarized as doing with the argument at index. |
| `LifecycleEvidence.CalleeSummarized` | lifecyclefacts | CalleeSummarized reports whether the call's static callee carries a lifecycle summary, so a consumer can distinguish a callee proven to do nothing with an argument from one it knows nothing about. |
| `LifecycleEvidence.ClosureHandsValueToUnreadableCallee` | lifecyclefacts | ClosureHandsValueToUnreadableCallee reports whether a literal passes the value it captured to a callee whose body this pass cannot read. |
| `LifecycleEvidence.ClosureRetainsValue` | lifecyclefacts | ClosureRetainsValue reports whether a function literal may keep the value it captured, judged by reading its body. |
| `LifecycleEvidence.CompletionOnEdge` | lifecyclefacts | CompletionOnEdge combines local and imported result-conditioned guarantees. |
| `LifecycleEvidence.ForCandidate` | lifecyclefacts | ForCandidate attributes the evidence traced from here on to candidate, so a trace selector retrieves the whole proof built for it. |
| `LifecycleEvidence.OwnedResult` | lifecyclefacts | OwnedResult reports whether the call's static callee is summarized as returning a struct that owns resource fields, and returns the methods of the result type whose ReleasedFields cover every owned field together with the index of that result. |
| `LifecycleEvidence.Prove` | lifecyclefacts | Prove returns one lifecycle proof with explicit provenance. |
| `LocalEvidence.Completion` | ssaflow |  |
| `LocalEvidence.OwnershipTransfer` | ssaflow | OwnershipTransfer proves and memoizes an ownership-transfer request. |
| `MayAliasAny` | ssaflow | MayAliasAny reports whether value may alias any candidate; see MayAlias. |
| `MayAliasThroughLoads` | ssaflow | MayAliasThroughLoads reports whether value may be target seen through transparent wrappers, loads, or a phi merge. |
| `MayAlias` | ssaflow | MayAlias reports a possible identity through conversions, any phi edge, and local storage history. |
| `MayContainValue` | ssaflow | MayContainValue reports whether owner may be an aggregate or closure that transitively contains value. |
| `MethodCallCoverage` | ssaflow | MethodCallCoverage reports whether calls holds over function's normal paths with the requested coverage. |
| `NamedType` | syntax | NamedType reports whether value names packagePath.name, allowing one pointer layer. |
| `NewCallEffects` | ssaflow | NewCallEffects constructs a query with a shared instruction budget. |
| `NewCallGraphMemo` | ssaflow |  |
| `NewFunctionSummaries` | ssaflow | NewFunctionSummaries fixes the computation and conservative fallback for a summary family. |
| `NewLifecycleEvidence` | lifecyclefacts | NewLifecycleEvidence constructs evidence whose accepted, rejected, and unknown results use the supplied analyzer identity for structured tracing. |
| `NewReachingWalk` | ssaflow | NewReachingWalk starts a fold that looks through forms. |
| `NewSearchBudget` | ssaflow | NewSearchBudget returns a budget allowing limit instructions. |
| `NewStorage` | ssaflow | NewStorage creates a bounded storage query using the caller's search budget. |
| `NormalReturnReachableFrom` | ssaflow | NormalReturnReachableFrom reports whether block can reach a normal return without first invoking a control-flow terminating API. |
| `PackageFunction` | syntax | PackageFunction identifies a package-level function. |
| `PackageMethod` | syntax | PackageMethod identifies the declared method. |
| `PackageVariable` | syntax | PackageVariable identifies a package-level variable. |
| `PhiEdgeCount` | ssaflow | PhiEdgeCount returns how many edges phi merges. |
| `PhiIncoming` | ssaflow | PhiIncoming yields each edge of phi with the predecessor block it comes from. |
| `Proof.Known` | ssaflow | Known reports whether available evidence proved or disproved the requested relationship. |
| `Proof.Proven` | ssaflow | Proven reports whether the requested relationship was established. |
| `ProveCompletionForResult` | ssaflow | ProveCompletionForResult summarizes exact parameter cleanup on normal returns matching predicate. |
| `ProveCompletionOnEdge` | ssaflow | ProveCompletionOnEdge asks whether the call result tested by from establishes completion of request.Target on the edge to to. |
| `ProveCompletion` | ssaflow | ProveCompletion answers one completion request. |
| `ProveEnclosingCompletion` | ssaflow | ProveEnclosingCompletion follows callback arguments from their lexical owner, requiring all discovered invocations to have the same cleanup guarantee. |
| `ProveIdentity` | ssaflow | ProveIdentity reports whether two values denote corresponding access paths beneath roots that the caller has already established as equivalent. |
| `ReachingWalk.Any` | ssaflow | Any reports whether some value reaching value satisfies leaf. |
| `ReachingWalk.EveryOf` | ssaflow | EveryOf reports whether every value in values satisfies leaf, judging each under its own visited set. |
| `ReachingWalk.Every` | ssaflow | Every reports whether every value reaching value satisfies leaf. |
| `ReachingWalk.Mark` | ssaflow | Mark records value as visited and reports whether this was its first visit. |
| `ResolveEmbeddedFieldPath` | ssaflow | ResolveEmbeddedFieldPath resolves an agreed embedded-field path through the walk's selected transparent forms. |
| `ResolveReachingValue` | ssaflow | ResolveReachingValue returns the one result that every value reaching value resolves to under leaf, where results agree when key maps them to the same key. |
| `ResolvedCallee` | ssaflow | ResolvedCallee returns the callee a call reaches, answering a generic instantiation with its origin. |
| `ResolvedFunction` | ssaflow | ResolvedFunction answers an instantiation with its origin for a function the caller already holds, such as the literal a launch names. |
| `ResourceCleanup` | lifecyclefacts | ResourceCleanup returns the cleanup methods of a resource type, or false when the type carries no obligation this vocabulary knows. |
| `ReturnedMayAliasAny` | ssaflow | ReturnedMayAliasAny reports whether a return may transfer any candidate value. |
| `ReturnedResult` | ssaflow | ReturnedResult returns the value a return statement hands back at index. |
| `ReturnedValueOwnsValueSummarized` | ssaflow | ReturnedValueOwnsValueSummarized is ReturnedValueOwnsValue for a caller that can answer for a callee whose body is unavailable. |
| `ReturnedValueOwnsValue` | ssaflow |  |
| `SameAccessPath` | ssaflow | SameAccessPath reports whether left and right select the same sequence of fields and constant indexes from their respective roots. |
| `SameExpression` | syntax | SameExpression reports whether two expressions identify the same syntactic value, using type information to distinguish identifiers with equal names. |
| `SearchBudget.Exhausted` | ssaflow | Exhausted reports whether the budget ran out, so a caller can trace the bailout and decline to retain an answer that was cut short. |
| `SearchBudget.Observed` | ssaflow | Observed attaches an observer that hears each give-up of a proof spending this budget, and returns the budget so a query can be built inline. |
| `SearchBudget.Spend` | ssaflow | Spend charges one instruction and reports whether the walk may continue. |
| `SelectedReceiveChannel` | ssaflow | SelectedReceiveChannel returns the channel necessarily received from before entering block. |
| `SelectedReceiveOnEdge` | ssaflow | SelectedReceiveOnEdge returns the channel received from when the exact select-case edge is taken. |
| `SendsValue` | ssaflow | SendsValue reports whether instruction hands value to a channel receiver. |
| `ShortPackageName` | syntax | ShortPackageName returns the final component of an import path. |
| `SourceRange` | syntax | SourceRange returns the smallest useful syntax range that starts at or contains position. |
| `SourceSSAFunctions` | ssaflow | SourceSSAFunctions returns non-generated source functions from buildssa results. |
| `SpawnInvokesArgumentOnEveryReturn` | ssaflow | SpawnInvokesArgumentOnEveryReturn reports whether the function launched by spawn invokes target synchronously before every normal return. |
| `SpawnedValueAtCall` | ssaflow | SpawnedValueAtCall resolves a spawned function value back to the value supplied by the parent goroutine instruction. |
| `Storage.Content` | ssaflow | Content returns the value agreed on by every reaching write. |
| `Storage.Projection` | ssaflow | Projection proves that value is a strict, non-empty access path from root whose selected storage cannot have been replaced before observation. |
| `Storage.Resolve` | ssaflow | Resolve follows loads at their own execution points, not at a later use. |
| `Storage.Same` | ssaflow | Same proves equality after resolving local loads. |
| `Storage.StableContent` | ssaflow | StableContent adds a lifetime check to Content: no other use may change the selected location after observation. |
| `StoredInto` | ssaflow | StoredInto yields every value stored into address, into a field or element selected from it, or through a pointer loaded from it. |
| `StoresOwnerOfValueInExternalField` | ssaflow | StoresOwnerOfValueInExternalField reports whether an aggregate containing value is installed on a receiver or caller-owned struct. |
| `StoresOwnerOfValueInField` | ssaflow | StoresOwnerOfValueInField reports whether instruction stores a callback or aggregate that transitively captures value into a struct field. |
| `StoresValueInEnclosingScope` | ssaflow | StoresValueInEnclosingScope reports assignment to a captured local owned by the enclosing synchronous caller. |
| `StoresValueInEscapingField` | ssaflow | StoresValueInEscapingField reports whether value is installed in a field of an owner that already outlives the function or is subsequently transferred. |
| `StoresValueInField` | ssaflow |  |
| `StoresValueInGlobal` | ssaflow | StoresValueInGlobal reports whether instruction transfers value into package-owned storage. |
| `StoresValueInOwnedMap` | ssaflow |  |
| `SuccessBranch` | ssaflow | SuccessBranch reports whether successor is the branch where errorValue is nil, when block ends in a recognizable nil comparison. |
| `Symbol.MatchesMethod` | syntax | MatchesMethod reports whether name is selected on the receiver identified by symbol. |
| `Symbol.MatchesObject` | syntax | MatchesObject reports whether object is the exact declaration identified by symbol. |
| `UnownedReturnAfterCallSuccess` | ssaflow | UnownedReturnAfterCallSuccess is UnownedReturn restricted to the branch on which call succeeded. |
| `UnownedReturnAssumingNonNilWithEdges` | ssaflow | UnownedReturnAssumingNonNilWithEdges adds edge-local ownership actions while preserving the same non-nil assumption and feasible-successor policy. |
| `UnownedReturnAssumingNonNil` | ssaflow | UnownedReturnAssumingNonNil is UnownedReturn with the additional fact that value is non-nil after start. |
| `UnownedReturnFromEntryAllow` | ssaflow | UnownedReturnFromEntryAllow reports whether any normal return lacks an ownership action unless allowReturn proves that return needs none. |
| `UnownedReturnFromEntryAssumingNonNil` | ssaflow | UnownedReturnFromEntryAssumingNonNil analyzes only paths feasible when value is non-nil at function entry. |
| `UnownedReturnFromEntryWithEdges` | ssaflow | UnownedReturnFromEntryWithEdges adds edge-local ownership actions to the ordinary entry-to-return query, including edges into shared successors. |
| `UnownedReturnWithEdges` | ssaflow | UnownedReturnWithEdges is UnownedReturn with edge-local ownership actions. |
| `UnownedReturn` | ssaflow | UnownedReturn reports whether any normal return reachable after start lacks an ownership action. |
| `Unparen` | syntax | Unparen removes every enclosing parenthesized expression. |
| `UnwrapTransparentValue` | ssaflow | UnwrapTransparentValue returns the operand of value only when its concrete SSA form is among forms. |
| `ValueCallsMethod` | ssaflow | ValueCallsMethod reports whether value is, or carries, a callback that calls method on target when invoked: a function literal whose body completes the target, a bound method value, or such a callback held in a local, passed through a call result, or merged by a phi. |
| `ValueDerivesFrom` | ssaflow | ValueDerivesFrom reports whether source contributes to value through SSA operands or a local load/store pair. |
| `ValueEscapes` | ssaflow | ValueEscapes reports whether value is transferred beyond its current function through a return, store, send, or escaping closure. |
| `ValueIsAccessPathFrom` | ssaflow | ValueIsAccessPathFrom reports whether value is root itself or a statically identifiable field or constant-index projection beneath root. |
| `ValueMatchesAnySymbol` | ssaflow | ValueMatchesAnySymbol reports whether value is one of the exact package declarations. |
| `ValueMatchesSymbol` | ssaflow | ValueMatchesSymbol reports whether value is the exact package declaration identified by symbol. |
| `WalkStates` | ssaflow | WalkStates drives a keyed work list over path-sensitive states. |
<!-- gohawk:generated-helpers:end -->
