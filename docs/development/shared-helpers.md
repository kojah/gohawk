---
title: Shared helpers
description: The ssaflow and lifecyclefacts helpers, indexed by the question each one answers.
sidebar:
  order: 1
---

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

## Cross-package lifecycle facts

| helper | answers |
|---|---|
| `lifecyclefacts.NewLifecycleEvidence` and `LifecycleEvidence.Prove` | the one decision path for consulting local evidence and then imported facts |
| `lifecyclefacts.ResourceCleanup(type)` | which methods release a resource of this type |
| `lifecyclefacts.CallReturnsView` | does the call return a view onto its argument rather than a new owner? |

Consumers go through `LifecycleEvidence`. Raw `analysis.Pass.ImportObjectFact`
and `analysis.Pass.ExportObjectFact` calls belong only in the package that
defines the fact type; `TestObjectFactsStayInTheirDefiningPackage` enforces it. See
[the fact model](../fact-model/) for what a fact can express.

## Adding a helper

Promote a mechanic here only after a second analyzer needs it. Make it
higher-order — it takes the leaf predicate — and form-selective — it takes the
transparent forms — so the next consumer reuses it instead of copying it.
Keep the doc comment as the contract: the exhaustive index in the codebase
skill (`.agents/skills/gohawk-codebase/references/shared-helpers.md`)
regenerates from it, and the architecture tests fail if a helper is missing
there. Then add the helper under the question it answers above.
