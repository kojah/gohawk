# Shared helpers

Shared APIs indexed by the question each answers, with generated package
references linked below. Consumer guidance and infrastructure contracts are
kept separate: an exported function is not necessarily an analyzer entrypoint.

Every analyzer stands on the shared engine in `internal/ssaflow`,
`internal/lifecycle`, and
`internal/passes/lifecyclefacts`. Before writing any traversal, provenance, or
ownership code, find the question below. The authoritative source is the doc
comment on each helper (`go doc ./internal/ssaflow`); this page indexes them by
purpose so the right one is easy to find.

Traversal helpers provide mechanics; summary components establish their own
domain guarantees. The decision to report a diagnostic stays beside each
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
| `EvaluateObligationWitness(ObligationFlow)` | the same, plus the violating return for a diagnostic to cite |
| `UnownedReturn`, `UnownedReturnFromEntryWithEdges`, `UnownedReturnFromEntryAllow` | is there a reachable normal return with no owning action before it? |
| `UnownedReturnAssumingNonNilWitness`, `UnownedReturnFromEntryAssumingNonNil` | the same, on paths feasible when a value is non-nil; the witness form returns the unowned return |
| `UnownedReturnAfterCallSuccessWitness` | the unowned return after a call's success branch, or nil |
| `NormalReturnReachableFrom` | can a normal return be reached from here? |
| `FeasibleSuccessors`, `SuccessBranch`, `BlockReachable`, `BlockInCycle` | control-flow feasibility primitives |
| `InstructionDominates`, `InstructionMayFollow`, `InstructionIndex` | ordering between instructions |
| `ProveCountedLoop` | exact bounded induction count and whether body operands depend on the counter; not termination or an unrolling policy |

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
| `heapmodel.QueryEscape(value, scope)` | structured confinement/escape evidence for a local allocation site, with destinations, origin instructions, and uncertainty |
| `SendsValue`, `ClosureCapturesValue`, `ValueHasTransferUse`, `ExternallyOwnedValue` | lifecycle-specific transfer uses; absence is not proof of confinement |
| `CallTransfersValueToField`, `CallTransfersArgumentToReturnedOwner`, `CallTransfersArgumentToReceiver`, `CallTransfersArgumentToLifecycleOwner` | ownership transfer through a call |
| `ReturnedValueOwnsValue`, `ReturnedMayAliasAny` | does a return carry the value or its owner? |
| `ClosureBindingPairs` | the captured variables of a closure paired with the values supplied for them |
| `CapturedBindingValue`, `CapturedBindingMatches` | inspect one captured binding |

Choose the escape scope explicitly. `EscapeFunction` includes result edges;
`EscapeBody` excludes them and cannot prove confinement beyond a normal return.
The latter supports iteration-local container checks. Events are may-path
observations, not unconditional effects or accepted cleanup obligations.
Only `EscapeLocal` proves confinement within the selected scope. Opaque calls,
foreign or merged identities, unknown contents, and budget cutoffs remain
unknown; do not convert them into ownership transfers.

## Which call is this?

| helper | answers |
|---|---|
| `CallEffects.Value`, `CallEffects.Call`, `CallEffectProof.PreservesStorage` | bounded local read/mutate/retain/async/invoke evidence, without ownership policy |
| `CallBindings`, `DirectCallee` | statically known callee and argument/capture mapping, without alias or completion claims |
| `ResolveInterfaceDispatch` | exact concrete receiver and method behind an interface box; not its effects or termination |
| `CallEffects.FieldCall`, `Storage.StableFieldContent` | selected embedded-slot effects and stable contents across visible helpers; not effects on the loaded object |
| `CallMatchesSymbol`, `CallMatchesAnySymbol`, `ValueMatchesSymbol`, `ValueMatchesAnySymbol` | does this call or value resolve to an exact well-known declaration? |
| `HasLibraryContract` | does this call match a registered external API contract? |
| `InstructionCall`, `CallName`, `CallReceiver`, `CallResult` | call metadata |
| `SourceSSAFunctions`, `FunctionFile` | source functions and their files |
| `InstructionTerminatesControlFlow`, `SpawnedValueAtCall`, `ChannelType`, `DefinitelyNil` | miscellaneous facts about instructions and values |

Match well-known functions through `syntax.Symbol`; do not reconstruct
identity from package paths and raw names.

## Match source and symbols

`internal/syntax` is the source-level layer: well-known declaration identity
for consumers of type information, and AST helpers shared by syntax-based
analyzers. Name-only matching is reserved for documented external contracts.

| helper | answers |
|---|---|
| `PackageFunction`, `PackageMethod`, `PackageVariable`, `Builtin` | build a `Symbol` for an exact declaration; compare its type object or use the SSA matchers above |
| `NamedType`, `IsErrorType` | named type identity and error interface implementation |
| `Unparen`, `ExpressionUsesObject` | parenthesis stripping and whether an expression reads an object |
| `GeneratedFile`, `SourceRange`, `AnalyzeFile`, `ShortPackageName` | skip generated files, recover a source range from a node, decide whether a file is analyzed, and abbreviate a package path for messages |

## Request function-summary knowledge

Start with [summaries](summaries.md) when a helper can hide evidence relevant
to a check. Declare `summaries.Select` once with the required result, lifecycle,
or concurrency components; use that selection for prerequisites and the provider.
Not requested, unavailable, and available-but-unknown are distinct states.

Use the [lifecyclefacts](lifecyclefacts.md), [concurrencyfacts](concurrencyfacts.md),
and [resultfacts](resultfacts.md) references to understand domain contracts or
change inference and cross-package publication. Analyzer access must still go
through the broker; the domain passes own raw fact import/export.

| helper | answers |
|---|---|
| `summaries.Provider.LifecycleEvidence` and `LifecycleEvidence.Prove` | obtain selected lifecycle evidence, then consult local and imported guarantees |
| `lifecyclefacts.ResourceCleanup(type)` | which methods release a resource of this type |
| `summaries.Provider.CallReturnsView` | does the call return a view onto its argument rather than a new owner? |

Consumers obtain `LifecycleEvidence` through the broker. Raw `analysis.Pass.ImportObjectFact`
and `analysis.Pass.ExportObjectFact` calls belong only in the package that
defines the fact type; `TestObjectFactsStayInTheirDefiningPackage` enforces it. See
[Inferred facts](../../../../docs/development/fact-model.md) for what a fact can express.

## Read a cross-package heap summary

Use [heapmodel](heapmodel.md) for per-function graph construction and queries,
heap-summary projection and registration, and call-site substitution. Its
contract names roots, slots, targets, effects, requirements, and truncation;
a missing edge is not proof of no alias when the slot was truncated.
[heapmodel](heapmodel.md) also owns demand-driven storage queries and their
graph fallbacks. [lifecycle](lifecycle.md) owns completion and transfer proofs
using that evidence. [ssaflow](ssaflow.md) owns the lower-level proof, budget, value,
call, and control-flow mechanics shared by both packages.

## Which external API changes resource state?

Use [resourcemodel](resourcemodel.md) for comparable per-path resource
obligations and exact external contracts such as a result-conditioned close.
It proves the target's identity and the API outcome; analyzer reporting policy
remains local. Lifecycle summaries can compose the contract through forwarding
helpers instead of repeating it at each caller.

## Adding a helper

Promote a mechanic here only after a second analyzer needs it. Make it
higher-order — it takes the leaf predicate — and form-selective — it takes the
transparent forms — so the next consumer reuses it instead of copying it.
Keep the doc comment as the contract: package references regenerate from it,
and the architecture tests fail if a helper is missing or stale. Then add the
helper under the question it answers above.

## Package references

Load only the package reference needed for the current task. These references
include exported types, constants, variables, functions, and methods with source
declarations and comments. They are regenerated by `go generate ./...`;
`TestSharedHelperReferencesStayCurrent` checks exact freshness without writing.

<!-- gohawk:generated-helpers:start -->
- [internal/heapmodel](heapmodel.md)
- [internal/lifecycle](lifecycle.md)
- [internal/passes/concurrencyfacts](concurrencyfacts.md)
- [internal/passes/lifecyclefacts](lifecyclefacts.md)
- [internal/passes/resultfacts](resultfacts.md)
- [internal/passes/testvariant](testvariant.md)
- [internal/resourcemodel](resourcemodel.md)
- [internal/ssaflow](ssaflow.md)
- [internal/summaries](summaries.md)
- [internal/syntax](syntax.md)
<!-- gohawk:generated-helpers:end -->
