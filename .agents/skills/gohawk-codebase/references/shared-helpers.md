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
| `ValueDerivesFrom`, `ValueAliases` | is this value a wrapped or merged form of that one? |
| `SameValue`, `SameAsAny`, `ValueContainsValue` | exact identity, including a value held inside an owner |
| `ProveIdentity`, `SameAccessPath`, `ValueIsAccessPathFrom` | structured identity of two access paths; is this value a field or index path from that root? |
| `UnmodifiedNonEmptyAccessPathAt` | is that access path still unmodified at an observation point? |
| `IdentitySource` | the identity-only source behind wrappers and loads (not for ownership) |
| `PhiIncoming`, `PhiMergesValue`, `PhiEdgeCount` | inspect one phi without walking it yourself |
| `UnwrapTransparentValue(value, forms)` | peel exactly the wrapper forms the caller selects |
| `ValueSources`, `ValuesShareErrorSource` | the source set behind a value; shared error provenance |

There is deliberately no universal unwrap helper. Callers select the exact
transparent forms that are sound for their proof and keep a fixture for a
wrapper that must stay opaque.

## Does an action cover every return?

The flow query behind classify-then-flow. Callers pass an `owns` predicate;
the helpers own feasibility and reachable-normal-return semantics.

| helper | answers |
|---|---|
| `UnownedReturn`, `UnownedReturnFromEntry`, `UnownedReturnFromEntryAllow` | is there a reachable normal return with no owning action before it? |
| `UnownedReturnAssumingNonNil`, `UnownedReturnFromEntryAssumingNonNil` | the same, on paths feasible when a value is non-nil |
| `UnownedReturnAfterCallSuccess` | the same, after a call's success branch |
| `ReachableReturns`, `NormalReturnReachableFrom` | which normal returns are reachable from here |
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
| `CallInvokesArgumentOnEveryReturn`, `DeferredClosureInvokesArgumentOnEveryReturn` | is the func argument itself invoked on every return? |
| `DeferredClosureCallsValue`, `ClosureCallsValue`, `CallReturnsDeferredCleanup` | deferred and callback cleanup shapes |

## Where did the value go?

Storage, escape, and transfer checks used by the classifiers to tell a
transfer from an opaque escape.

| helper | answers |
|---|---|
| `StoresValueInField`, `StoresValueInGlobal`, `StoresValueInEnclosingScope`, `StoresValueInEscapingField`, `StoresValueInOwnedMap` | where a store puts the value |
| `StoresOwnerOfValueInField`, `StoresOwnerOfValueInExternalField` | a store of the value's owner |
| `SendsValue`, `ClosureCapturesValue`, `ValueEscapes`, `ExternallyOwnedValue` | sends, captures, and escapes |
| `CallTransfersValueToField`, `CallTransfersArgumentToReturnedOwner`, `CallTransfersArgumentToReceiver`, `CallTransfersArgumentToLifecycleOwner` | ownership transfer through a call |
| `ReturnedValueOwnsValue`, `ReturnSameValue`, `ReturnedSameAsAny` | does a return carry the value or its owner? |
| `ClosureBindingPairs` | the captured variables of a closure paired with the values supplied for them |
| `CapturedBindingValue`, `CapturedBindingMatches` | inspect one captured binding |

## Which call is this?

| helper | answers |
|---|---|
| `CallMatchesSymbol`, `CallMatchesAnySymbol`, `ValueMatchesSymbol`, `ValueMatchesAnySymbol` | does this call or value resolve to an exact well-known declaration? |
| `HasLibraryContract` | does this call match a registered external API contract? |
| `InstructionCall`, `CallName`, `CallPackage`, `CallReceiver`, `CallResult` | call metadata |
| `StaticCallsites`, `StaticCalls`, `SourceSSAFunctions`, `FunctionFile` | call graphs and source functions |
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
| `NamedType`, `IsErrorType`, `IsStringType`, `ParameterTypes` | type predicates that look through pointers and named types |
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
| `CallGraphMemo.Answer` | ssaflow | Answer returns the memoized answer for key, computing it once. |
| `CallGraphMemo.Cut` | ssaflow | Cut records that an answer was shortened for a reason of the caller's own, such as an exhausted budget, so that answer is not retained either. |
| `CallGraphMemo.Enter` | ssaflow | Enter marks function as being on the current path. |
| `CallGraphMemo.Entered` | ssaflow | Entered reports whether function is already on the current path, for a caller that must answer a cycle differently from the value Enter implies. |
| `CallGraphMemo.Leave` | ssaflow | Leave un-marks function as the walk returns past it. |
| `CallInvokesArgumentOnEveryReturn` | ssaflow |  |
| `CallMatchesAnySymbol` | ssaflow | CallMatchesAnySymbol reports whether common statically resolves to one of symbols. |
| `CallMatchesSymbol` | ssaflow | CallMatchesSymbol reports whether common statically resolves to symbol. |
| `CallName` | ssaflow | CallName returns a statically known method, function, or builtin name. |
| `CallPackage` | ssaflow | CallPackage returns a statically known package path for reporting, configuration, or package-wide API families. |
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
| `ClosureBindingPairs` | ssaflow |  |
| `ClosureCallsValue` | ssaflow | ClosureCallsValue reports whether a call-like closure or created callback calls target. |
| `ClosureCapturesValue` | ssaflow | ClosureCapturesValue reports whether instruction creates a closure that owns value. |
| `DeferredClosureCallsValue` | ssaflow | DeferredClosureCallsValue reports whether a deferred closure calls target. |
| `DeferredClosureInvokesArgumentOnEveryReturn` | ssaflow | DeferredClosureInvokesArgumentOnEveryReturn reports whether a deferred closure delegates target to a helper that invokes it on every normal path. |
| `DefinitelyNil` | ssaflow | DefinitelyNil reports whether every represented SSA value is nil. |
| `ElementOfAggregate` | ssaflow | ElementOfAggregate reports whether value was selected from an element of a slice, array, map, or range iteration, directly or through fields and loads. |
| `ExpressionUsesObject` | syntax | ExpressionUsesObject reports whether node refers to object. |
| `ExternallyOwnedValue` | ssaflow | ExternallyOwnedValue reports whether value comes from storage that outlives the current function invocation. |
| `Fact.AFact` | lifecyclefacts |  |
| `Fact.DescribeFact` | lifecyclefacts | DescribeFact renders the summary for the fact dump: one line per parameter that some mask covers, named from the function's signature. |
| `Fact.MethodMask` | lifecyclefacts | MethodMask selects the parameter mask for a lifecycle method. |
| `Fact.String` | lifecyclefacts | String decodes the masks by parameter position so the fact is readable in analysis debug output. |
| `FeasibleSuccessors` | ssaflow | FeasibleSuccessors preserves constants selected by predecessor-sensitive phis. |
| `FunctionFile` | ssaflow | FunctionFile returns source file containing function. |
| `FunctionParameterObject` | syntax | FunctionParameterObject returns the declared object at the positional parameter index. |
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
| `IsStringType` | syntax | IsStringType reports whether value has string as its underlying type. |
| `LifecycleEvidence.ArgumentRetainedByCallee` | lifecyclefacts | ArgumentRetainedByCallee reports whether the call's static callee is summarized as keeping the argument that contains target somewhere other than its returned value: a logger sink, a registry, a receiver field. |
| `LifecycleEvidence.ArgumentRetained` | lifecyclefacts | ArgumentRetained reports whether the summary of the call's static callee marks the argument at index as retained. |
| `LifecycleEvidence.ArgumentReturnedAsView` | lifecyclefacts | ArgumentReturnedAsView reports whether the call's static callee is summarized as returning a view over the argument that contains target: the argument is stored in the returned struct and nothing on that type releases it. |
| `LifecycleEvidence.CalleeSummarized` | lifecyclefacts | CalleeSummarized reports whether the call's static callee carries a lifecycle summary, so a consumer can distinguish a callee proven to do nothing with an argument from one it knows nothing about. |
| `LifecycleEvidence.ForCandidate` | lifecyclefacts | ForCandidate attributes the evidence traced from here on to candidate, so a trace selector retrieves the whole proof built for it. |
| `LifecycleEvidence.Identity` | lifecyclefacts | Identity proves a local access-path relationship through the same memoized evidence and tracing channel used for lifecycle evidence. |
| `LifecycleEvidence.OwnedResult` | lifecyclefacts | OwnedResult reports whether the call's static callee is summarized as returning a struct that owns resource fields, and returns the methods of the result type whose ReleasedFields cover every owned field together with the index of that result. |
| `LifecycleEvidence.Prove` | lifecyclefacts | Prove returns one lifecycle proof with explicit provenance. |
| `LocalEvidence.Completion` | ssaflow |  |
| `LocalEvidence.Identity` | ssaflow |  |
| `LocalEvidence.OwnershipTransfer` | ssaflow | OwnershipTransfer proves and memoizes an ownership-transfer request. |
| `MethodCallCoverage` | ssaflow | MethodCallCoverage reports whether calls holds over function's normal paths with the requested coverage. |
| `NamedType` | syntax | NamedType reports whether value names packagePath.name, allowing one pointer layer. |
| `NewCallGraphMemo` | ssaflow |  |
| `NewLifecycleEvidence` | lifecyclefacts | NewLifecycleEvidence constructs evidence whose accepted, rejected, and unknown results use the supplied analyzer identity for structured tracing. |
| `NewReachingWalk` | ssaflow | NewReachingWalk starts a fold that looks through forms. |
| `NormalReturnReachableFrom` | ssaflow | NormalReturnReachableFrom reports whether block can reach a normal return without first invoking a control-flow terminating API. |
| `PackageFunction` | syntax | PackageFunction identifies a package-level function. |
| `PackageMethod` | syntax | PackageMethod identifies the declared method. |
| `PackageVariable` | syntax | PackageVariable identifies a package-level variable. |
| `ParameterTypes` | syntax | ParameterTypes expands a field list into one entry per declared parameter. |
| `PhiEdgeCount` | ssaflow | PhiEdgeCount returns how many edges phi merges. |
| `PhiIncoming` | ssaflow | PhiIncoming yields each edge of phi with the predecessor block it comes from. |
| `PhiMergesValue` | ssaflow | PhiMergesValue reports whether some edge of phi is value under SameValue. |
| `Proof.Known` | ssaflow | Known reports whether available evidence proved or disproved the requested relationship. |
| `Proof.Proven` | ssaflow | Proven reports whether the requested relationship was established. |
| `ProveCompletion` | ssaflow | ProveCompletion answers one completion request. |
| `ProveIdentity` | ssaflow | ProveIdentity reports whether two values denote corresponding access paths beneath roots that the caller has already established as equivalent. |
| `ReachableReturns` | ssaflow | ReachableReturns returns normal returns reachable after start. |
| `ReachingWalk.Any` | ssaflow | Any reports whether some value reaching value satisfies leaf. |
| `ReachingWalk.EveryOf` | ssaflow | EveryOf reports whether every value in values satisfies leaf, judging each under its own visited set. |
| `ReachingWalk.Every` | ssaflow | Every reports whether every value reaching value satisfies leaf. |
| `ReachingWalk.Mark` | ssaflow | Mark records value as visited and reports whether this was its first visit. |
| `ResolveReachingValue` | ssaflow | ResolveReachingValue returns the one result that every value reaching value resolves to under leaf, where results agree when key maps them to the same key. |
| `ResourceCleanup` | lifecyclefacts | ResourceCleanup returns the cleanup methods of a resource type, or false when the type carries no obligation this vocabulary knows. |
| `ReturnSameValue` | ssaflow | ReturnSameValue reports whether a return transfers value. |
| `ReturnedResult` | ssaflow |  |
| `ReturnedSameAsAny` | ssaflow | ReturnedSameAsAny reports whether a return transfers any candidate value. |
| `ReturnedValueOwnsValue` | ssaflow |  |
| `SameAccessPath` | ssaflow | SameAccessPath reports whether left and right select the same sequence of fields and constant indexes from their respective roots. |
| `SameAsAny` | ssaflow | SameAsAny reports whether value aliases any candidate. |
| `SameExpression` | syntax | SameExpression reports whether two expressions identify the same syntactic value, using type information to distinguish identifiers with equal names. |
| `SameValue` | ssaflow | SameValue reports SSA identity through conversions, phis, and local load/store pairs. |
| `SendsValue` | ssaflow | SendsValue reports whether instruction hands value to a channel receiver. |
| `ShortPackageName` | syntax | ShortPackageName returns the final component of an import path. |
| `SourceRange` | syntax | SourceRange returns the smallest useful syntax range that starts at or contains position. |
| `SourceSSAFunctions` | ssaflow | SourceSSAFunctions returns non-generated source functions from buildssa results. |
| `SpawnedValueAtCall` | ssaflow | SpawnedValueAtCall resolves a spawned function value back to the value supplied by the parent goroutine instruction. |
| `StaticCalls` | ssaflow | StaticCalls indexes ordinary statically resolved calls by their callee. |
| `StaticCallsites` | ssaflow | StaticCallsites indexes every statically resolved call-like instruction by its callee. |
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
| `UnmodifiedNonEmptyAccessPathAt` | ssaflow | UnmodifiedNonEmptyAccessPathAt reports whether value is a strict, non-empty access path from root whose selected storage cannot have been replaced before observation. |
| `UnownedReturnAfterCallSuccess` | ssaflow | UnownedReturnAfterCallSuccess is UnownedReturn restricted to the branch on which call succeeded. |
| `UnownedReturnAssumingNonNil` | ssaflow | UnownedReturnAssumingNonNil is UnownedReturn with the additional fact that value is non-nil after start. |
| `UnownedReturnFromEntryAllow` | ssaflow | UnownedReturnFromEntryAllow reports whether any normal return lacks an ownership action unless allowReturn proves that return needs none. |
| `UnownedReturnFromEntryAssumingNonNil` | ssaflow | UnownedReturnFromEntryAssumingNonNil analyzes only paths feasible when value is non-nil at function entry. |
| `UnownedReturnFromEntry` | ssaflow | UnownedReturnFromEntry reports whether any normal return lacks an ownership action. |
| `UnownedReturn` | ssaflow | UnownedReturn reports whether any normal return reachable after start lacks an ownership action. |
| `Unparen` | syntax | Unparen removes every enclosing parenthesized expression. |
| `UnwrapTransparentValue` | ssaflow | UnwrapTransparentValue returns the operand of value only when its concrete SSA form is among forms. |
| `ValueAliases` | ssaflow | ValueAliases reports whether value is a wrapped or phi-derived form of target. |
| `ValueCallsMethod` | ssaflow | ValueCallsMethod reports whether value is, or carries, a callback that calls method on target when invoked: a function literal whose body completes the target, a bound method value, or such a callback held in a local, passed through a call result, or merged by a phi. |
| `ValueContainsValue` | ssaflow |  |
| `ValueDerivesFrom` | ssaflow | ValueDerivesFrom reports whether source contributes to value through SSA operands or a local load/store pair. |
| `ValueEscapes` | ssaflow | ValueEscapes reports whether value is transferred beyond its current function through a return, store, send, or escaping closure. |
| `ValueIsAccessPathFrom` | ssaflow | ValueIsAccessPathFrom reports whether value is root itself or a statically identifiable field or constant-index projection beneath root. |
| `ValueMatchesAnySymbol` | ssaflow | ValueMatchesAnySymbol reports whether value is one of the exact package declarations. |
| `ValueMatchesSymbol` | ssaflow | ValueMatchesSymbol reports whether value is the exact package declaration identified by symbol. |
| `ValueSources` | ssaflow |  |
| `ValuesShareErrorSource` | ssaflow |  |
| `WalkStates` | ssaflow | WalkStates drives a keyed work list over path-sensitive states. |
<!-- gohawk:generated-helpers:end -->
