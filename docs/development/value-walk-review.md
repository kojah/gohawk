# Shared value-walk review

This review covers production Go files directly in `internal/ssaflow` and
`internal/lifecycle`, as of the `gohawk-dho.21` consolidation. It does not
certify the entire architecture or remove any audited false-positive sites.
Graph tools were unavailable. An AST scan identified same-name calls, then
source inspection distinguished recursion from unrelated method calls.
Searches for SSA-value visited maps also covered mutually recursive walks.
The inventory tool and parent comparison overlay are local `.build` artifacts;
the source contracts and committed tests below are the durable evidence.

## Consolidated mechanics

| Query | Transitions and cycle policy | Shared driver and validation |
| --- | --- | --- |
| `ssaflow.DerivesFrom` | Check the caller's identity predicate before expanding arbitrary instruction operands. Loads also follow exact stores and eligible whole-aggregate stores. Revisited values add no evidence. | `WalkStates`, keyed by SSA value. [TestDerivationBoundaries](../../internal/ssaflow/value_derivation_test.go) covers opaque calls, arithmetic, boxing, stores, unrelated allocations and loops; [TestDerivationIdentityBeforeOperands](../../internal/ssaflow/value_derivation_test.go) covers identity of an empty/cyclic phi and cycles with/without a source. Both pass before and after consolidation. |
| `lifecycle.exactCallbacks` | Selected wrappers, stable storage at the invocation, and permitted `sync.OnceFunc` arguments. Every phi alternative must resolve; sibling branches are independent. Empty merges, cycles and opaque alternatives fail and discard partial results. | `ReachingWalk.Every`. [TestExactCallbackAlternatives](../../internal/lifecycle/completion_callbacks_test.go) covers shared branches, nested merges, cycles and partial failure; parent overlay and current implementation agree. Existing [TestCompletionBoundaries](../../internal/lifecycle/completion_search_test.go) covers launch forms, storage mutation, cleanup coverage and OnceFunc restrictions. |
| `lifecycle.valueOwnsValue` | Check possible alias before selected wrappers; then follow closure captures with the existing capture-identity/content policy. No independent phi fan-out or call-result traversal. Revisited owners add no evidence. | `WalkStates`, keyed by owner. [TestStoredCallbackOwnershipBoundary](../../internal/lifecycle/store_capture_policy_test.go) preserves direct/nested callbacks, unrelated captures, aggregate captures and the distinction from broader containment. |

The work-list changes replace depth-first existential searches with keyed
work lists. Their predicates and transitions do not depend on visit order;
they have no search budget or observation changes. They remain possible
provenance/ownership evidence, never must-cleanup guarantees. Callback
resolution retains its branch order and duplicate closure entries.

## Distinct walks retained

| Candidate | Transition and termination policy; reason to retain its own model |
| --- | --- |
| `sameValueSeen`, `sameWrappedValue`, `storedValueMatches` | Directional paired identity, wrappers on both sides, matching field/index bases, load addresses and exact cell stores, with existential phi alternatives. Each directional query tracks visited left values; direct equality precedes the guard. Index comparisons start separate identity queries. A single-value reaching fold would lose the changing target and paired comparisons. |
| `StructurallyIdentical` | Already folds reaching alternatives through `ReachingWalk.Every`; recursive field/index comparisons descend paired address structure. It is not arbitrary operand derivation. |
| `accessPathSteps` | Exact root identity before wrappers, then field/constant-index/load descent producing an ordered access path. A visited value rejects a cyclic path. A Boolean fold cannot preserve the path result. |
| `addressOnlyLoaded` | Every referrer must be a load, debug reference, or another field/index selection with the same property. Recursion descends selection structure; other uses fail. It does not follow general value operands or phi edges. |
| `CallEffects.uses` / `use` | Aggregate effects over forward uses, including reads, mutation, retention, async use and unknown. Revisited values contribute neutral effects; each use spends a shared budget and exhaustion adds unknown. Function recursion uses the separate call memo. A short-circuit existence fold would change effects or budget semantics. |
| `SliceVersions`, `ChannelValues` | Work queues enumerate distinct aliases in encounter order, using membership sets. Their selected forwarding/append/channel-use rules and collection outputs differ from a Boolean reaching query. |
| `appendClosures`, `capturedReadOnly`, `closureCallsCapturedValue` | Descend the lexical anonymous-function tree, carrying capture bindings/predicates; captured-read-only checks reject writes and opaque uses. Lexical children terminate this traversal. They are not SSA-value graph walks. |
| `GuardCondition`, `completionTest` | Decode Boolean negation and supported comparison forms while carrying polarity and stability/outcome information. Recursion strips syntax rather than expanding merges; unsupported forms decline. |
| `GuardAddressIdentity` | Descend selected address/load forms to build a stable identity string; unsupported forms decline. Recursion follows address structure, not all reaching values. |
| `NaturalLoop.invariant` | Accept values defined outside the loop and descend the argument of permitted fixed-length builtin `len` calls. Unsupported loop-local computations fail; operand structure bounds this recursion. |
| `resolveCallbackBinding` | Binding substitution precedes wrappers; field/element and stable-cell resolution retain a binding environment and observation. Each step spends the shared budget and uses `ReachingWalk.Mark` for its cycle guard. It deliberately declines unsupported merges. |
| `enclosingSearch.resolve`, `readOnly` | Resolve across binding environments and observation-time storage, spending the shared budget. Read-only checks traverse selected fields and visible callees under the function memo; unresolved frames and exhausted searches decline. A value-only state key is insufficient. |
| `exactCleanupReceiver` | Strip only selected wrappers before exact parameter comparison. The wrapper chain terminates; no load, phi or aggregate containment is admitted. |
| `completionSearch.valueCallsMethod` | Possible callback-carried cleanup through wrappers, local stores, call arguments and phi alternatives. Revisited values mark the completion memo incomplete; closure bodies use its coverage proof. A generic fold's neutral cycle result would lose that memo effect. |
| `ownershipSearch.aggregateStoresValue` | Possible containment through local constructors, summaries, captures, loads, stores and phi alternatives. The visited key is `(aggregate, target)` because callee binding changes the target. Revisited pairs add no evidence. A single-value fold would conflate different ownership questions. |
| `storedInto` | Stream exact stores while descending field/index selections and pointer loads in depth-first referrer order. The visited-address set skips repeats, and a consumer can stop immediately. A breadth-first Boolean work list would change enumeration and early-stop behavior. |

The name-based AST inventory also reported `SearchBudget.Spend`,
`NaturalLoop.Contains`, and `completionSearch.receives`. Source inspection
shows delegation to a parent budget, `slices.Contains`, and a different
receiver's `receives` method respectively. They are not recursive SSA walks.
Callback binding maps are environments, not duplicated visited sets.

These distinctions preserve evidence policy; they do not prohibit later
consolidation. Any replacement must retain paired/frame state, output order,
memo incompleteness, and budget effects where applicable. This review does
not justify a universal unwrap or escape helper.
