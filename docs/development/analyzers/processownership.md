# processownership design notes

The public page is [processownership](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

Fire-and-forget alone does not establish a defect: opening a browser or
relaunching a process may intentionally outlive the caller. The former
`detached` audit is retired; such launches remain outside `missing-wait`.
Reading a field of the handle, such as logging the child's PID, does not
count as handing it on. The `missing-wait` check reports handles that are
waited on or released on some paths but not all.

Synchronous `Read`, `Write`, and `Close` on exact `exec.Cmd` standard IO pipe
results do not hand on a process wait owner. Their returned errors and counts
also carry no wait handle. A launch using only these operations retains the
unused-command unknown outcome; it may still leak a child and is an accepted
coverage gap. This covers the [rev-dep telemetry launch](https://github.com/jayu/rev-dep/blob/8a2fdb0927e2fc9b2a5b178c94f55d1887659152/internal/telemetry/telemetry.go#L75-L103).
Returning a pipe, handing it to other code, or launching its operations in a
goroutine retains the ordinary ownership question. Partial waits, direct
process operations and misleading project-defined pipe methods remain checked.
`pipe_handles.go` pins local input/output/error IO and partial waits beside
Kill and unrelated methods; `orphanedPipe` retains the returned-pipe diagnostic.
The final trace tests require unused-command unknown without budget exhaustion.

The post-Start reporting decision includes that unused-handle boundary before
emitting its final trace. An uncovered return whose command is unused produces
`unused-command-ownership-unknown`, rather than a rejected `unowned-return`
decision followed by silent suppression. Reporting and tracing consume the
same structured decision; this changes trace accuracy, not diagnostics.
The browser-launch fixture and trace assertion pin this distinction alongside
accepted waits, opaque handoffs and reported partial waits.

A command captured by a callback passed to an opaque runner is an uncertain
handoff, not a proven leak or a guaranteed wait. The same rule applies when
that runner is launched with `go`, including an imported panic-reporting
wrapper whose invocation guarantee is unavailable. A visible runner that
drops the callback, a callback holding another command, and a return before
the handoff retain the diagnostic (`opaque_waiters.go`). Both ordinary and
launched calls use one opaque-callback decision. The
[rune waiter](https://github.com/unstablebuild/rune/blob/3e2165f8983280542c985947378dfa740a397d03/internal/workspace/file_scheme.go#L458-L467)
is the representative imported-wrapper shape. A launched waiter with an
explicit `Wait` followed by process termination is also outside the normal-return
completion proof. Locally stored command fields are resolved at acquisition when
checking whether a returned value owner contains the command.

A returned aggregate may instead retain `cmd.Process`, leaving `exec.Cmd` local.
The process-handle classifier asks the existing returned-value containment query
about handle loads derived from the started command. Such a return is an unknown
handoff, not a guaranteed wait: the owner may never perform cleanup. This covers
the [sandbox container](https://github.com/criyle/go-sandbox/blob/6a60e40be9d0cefb656c4ae12415c5fd040df954/container/environment_linux.go#L266-L280),
whose `Destroy` method waits on its stored handle. The loop over candidate loads
uses one query budget from the command's pool. `returned_handles.go` covers a
returned owner and slice, a different process, a PID-only result, and a return
that bypasses transfer. Discarded local owners remain diagnostic. As elsewhere,
possible containment may lose real leaks; it never establishes completion.

Commands supplied by helpers, including `(command, error)` factories and
interface calls, have uncertain ownership. The check does not assume their
caller is the only possible wait owner. This can miss genuine leaks when a
factory actually returns an exclusively owned command; it is not proof that
the factory registered cleanup. Direct `exec.Command` and `exec.CommandContext`
acquisitions remain checked.

A `Wait` receiver merged from several commands is also uncertain when one
possible receiver is the started command. This avoids inventing a missing wait
when a failed `Start` selects a fallback command. It does not prove that a
merged receiver chooses the correct process; that ambiguity is an accepted
coverage gap. Returns before the possible wait and waits on an unrelated
command remain checked.

An immediate, non-cyclic merge on the successful `Start` edge can preserve
exact command identity. Its later `command != nil` guard is then known true
for this acquisition; branches that never started a command do not create a
missing wait. Replacing that merged value with nil does not preserve this fact.
A deferred wait guarded only by the captured command's `Process` field remains
uncertain when distinct loads prevent an exact identity proof. Additional
Boolean guards and visible field replacements inside the deferred waiter still
require a wait.

An immediate four-instruction guard after a checked successful `Start` reads
the exact command's `Process` without an intervening call or write. That load
is fixed non-nil in the shared flow's assumptions, so a conditional `Release`
or `Wait` can join a merged return without inventing an unowned nil path.
This replaces the separate immediate nil-return exception; the trace records
`successful-start-process-non-nil` as evidence used by the flow.
[Tencent's detached bus](https://github.com/TencentCloud/tencentmeeting-cli/blob/e631b355da2b001d24b82f453b65d96f39c59865/internal/event/spawner/spawner.go#L113-L123)
uses this form. Only that load gets the assumption: later loads, field
replacement, another command, and an additional Boolean guard remain outside
the guarantee. `process_guards.go` pins both merged and direct returns beside
these diagnostic controls.

## Result feasibility

The post-Start walk uses `summaries.Provider.Successors`, through the shared
`UnownedReturnQuery.Successors` adapter, just as sibling lifecycle checks use
the provider's feasible edges. Completed result contracts can exclude an
impossible return path; they do not establish Wait, Release or a transfer.
The provider retains ordinary successors for unknown results or interrupted
inference. Its existing result-pair rules also remain authoritative; this
consumer introduces no argument- or receiver-state inference.

`result_feasibility.go` pins imported always-nil and always-true helpers, a
local always-false helper, and a command merged on Start failure. Variable
errors, unresolved interface calls, and boxed typed-nil errors retain the
missing-wait diagnostic. Both the direct successful-Start and merged-command
entry paths use the same successor hook and retain their existing assumptions.
The provider bounds each feasibility query separately; this does not establish
a bound for the entire process obligation walk.

This does not resolve the
[Ferro Start error branch](https://github.com/ferro-labs/ai-gateway/blob/d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4/mcp/stdio.go#L154-L166).
Its dependency's Stdio.Start has no unconditional successful-result contract;
proving that branch impossible needs constructor-dependent receiver state.

## One-time program-entry ownership

An uncovered return after a one-time Start in the executable's real entry is
program-lifetime ownership unknown. `decideProcessReturn` uses the shared
`calls.RunsOnceInProgramEntry` structural proof after exact wait/transfer and
unused-command decisions. This boundary publishes no callee guarantee and does
not equate Kill with Wait, guarantee child termination on parent exit, or prove
that the parent observed the child's result. Missing observation or resources
retained until exit can still be defects; this is accepted coverage loss in an
intent-sensitive experimental audit.

A Start in a cycle, a referenced entry function, a reusable helper, a method,
a closure, or a function named main in another package does not qualify. The
shared entry proof now checks the synthetic initializer as well as declared
functions, so package-level function aliases and tables decline the guarantee.
`program_entry_test.go` asserts unknown versus exact settlement and preserves
reports across all those rejected forms. The `processentry`, `processentryloop`
and `processentryreferenced` fixtures check the full reporting pipeline. Shared
initializer-reference controls also check an unrelated function alias.

[coder's example agent](https://github.com/coder/acp-go-sdk/blob/0845a3bb9eddda5bfc22a94dd3598c90cb842451/example/agent/main.go#L401-L423)
starts one child in main, then returns after its explicit Kill. The three sibling
examples have the same entry horizon. Their CommandContext construction and
cleanup intent motivate uncertainty; neither a project name nor that constructor
establishes a Wait guarantee. The final trace reason is
`program-lifetime-ownership-unknown`, with outcome unknown.

## Tier

`missing-wait` moved from core to experimental on 2026-09-27. In batches 62
and 63 it had 10 true positives and 9 false positives, and in production
files, which is what a default run analyzes, 7 and 8. The false positives
were children deliberately detached with `Process.Release` or `Setsid`,
children started in `main` of example programs, whose exit ends the
program, and a process stored in a returned container that waits on
destroy. Whether a started child is meant to outlive its launcher is intent
the code rarely states, so the check stays available but is not run by
default.


## Deferred waiter discovery and coverage allowance

Deferred closure waiters have one completed census of callable instructions,
stores and loads, shared across captured and supplied command queries. The
successful-Start non-nil Process assumption belongs to this analyzer; it does
not make arbitrary loaded receiver identities equal. Capture proofs retain
priority over argument proofs. The census, binding visits, selected witnesses
and shared every-return coverage query use the command candidate's child
allowance. Any interrupted stage supplies unknown, never exact cleanup or
absence of ownership. Capture and argument metadata are yielded lazily by
`ClosureBindingPairsWithin` and `CallBindingsWithin` under that same allowance.
An interrupted capture census returns unknown before argument fallback, and an
interrupted argument census returns unknown before final disproof. Heap
identity/type queries retain their existing separate cost boundaries.

`deferred_wait_test.go` uses compiled SSA to pin exact, conditional, guarded
and replaced Process forms, intermediate cutoffs and fresh-child recovery.
`deferred_binding_budget_test.go` adds an argument-heavy compiled closure:
cutoff remains unknown with its parent pool available, and a fresh query proves
the supplied-command Wait. The old and new query both complete at 86 visits;
the change removes eager binding slices without claiming extra charged work.
The classifier and coverage controls reject exhausted candidate/query budgets.
Existing fixture pairs in `processownership.go` and `guarded_merge.go` preserve
accepted defensive guards and diagnostic independent flags or field replacement.


## Opaque callback choices

A phi choice of closures passed to an opaque runner or launched dynamically
can retain the started command. `lifecycle.ProvePossibleClosureCaptureWithin`
checks possible captures under the candidate allowance. The process adapter
maps positive capture to unknown Wait participation and never
proves a unique target, callback invocation or exact Wait. Conversions and loads
stay opaque. Known-body runners continue through ordinary completion evidence;
opaque handoff classification does not excuse a visible callback dropper.

The handoff proof now exposes state and reason rather than a Boolean. The
existing nonreturning spawned-waiter branch uses bounded normal-return discovery
and bounded completion under the same allowance. Both capture and return-query
cutoffs remain unknown. Graph/type/alias internals retain independent costs.
`processchoices/choices.go` pins accepted mixed captures and dynamic launches,
with unrelated commands, early bypass returns and converted callables diagnostic.
`internal/engine/lifecycle/completion_callbacks_test.go` checks actual SSA, negative captures
and fresh allowance recovery. `handoff_test.go` retains nonreturning waiter
allowance controls.


## Shared pre-Start instruction census

The cleanup-registration, wrapper-owner and external-store policies consume
one completed census of instructions strictly dominating Start. Shared SSA
traversal preserves function block order and excludes Start and its later
same-block instructions, including loop bodies. A child cutoff discards the
prefix and traces unknown ownership before reporting can use it. The policies
retain their distinct method sets, owner/watcher requirements and destination
rules; the census does not publish unconditional cleanup facts.
`prestart_test.go` pins intermediate cutoffs and fresh child recovery. Owner
arguments and result references share the census allowance; watcher discovery
and containment use a candidate child allowance. Heap, type and symbol-query
internals retain independent costs.

## Startup owner and successful-return queries

Only result types capable of retaining references enter the wrapper-owner
inventory. Void calls, scalar-only SSA tuples and scalar projections cannot
own the command. Mixed tuples keep the tuple and their reference-capable
projections; pointers, interfaces, functions and reference-bearing aggregates
remain eligible. This avoids asking completion about the nonexistent result of
[Ferro's configuration helper](https://github.com/ferro-labs/ai-gateway/blob/d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4/mcp/stdio.go#L135).

Result filtering does not erase instruction effects. A dominating helper with
no reference-bearing result can still retain or asynchronously expose the
command. The existing bounded local call-effect query makes such participation
unknown, including unavailable bodies and cutoffs; known reads and configuration
writes alone leave the obligation local. Possible retention never proves Wait.
`prestart_values.go` pairs a void registration with a non-retaining scalar helper
and a conditional Wait. `prestart_values_test.go` pins actual SSA result shapes,
configuration writes and complete versus interrupted call-effect answers.

Registered wrapper candidates are part of the completed pre-Start inventory.
Argument and result-referrer visits share that census allowance; cutoff discards
both instruction and owner prefixes. Later watcher discovery uses a bounded
body traversal and bounded containment, retaining its literal-closure and
source-position policy. A capture is possible supervision, never exact Wait.
The enclosing cleanup-registration policy keeps that uncertainty distinct from
unconditional callee guarantees.

Wrapper supervision submits only deferred launches to completion: `defer` and
synchronous registrations with the exact `testing.Cleanup` contract. Ordinary
calls, including a helper with its own defer, complete in the helper's scope;
goroutine launches have a separate completion reason. Neither can supply the
deferred-completion reason accepted by this startup policy. Skipping those
queries avoids spending allowance mapping every earlier helper against every
owner result. The command's own cleanup/transfer questions remain separate.
No filter uses value definition order, since a later result may alias an already
existing object. `startup_queries_test.go` pins all launch forms, a missing
watcher, retained `testing.Cleanup` supervision and deferred cutoff uncertainty.

Successful-Start return reachability has a structured result. Proven no-return
branches can end startup analysis; a cutoff leaves ownership unknown and cannot
be inverted into a no-return guarantee. The ordinary success-branch selector,
termination catalogue and unrelated owner policies retain their meanings.
`startup_queries_test.go` compiles literal watcher, early/unrelated watcher,
returning, looping and panicking branches and exercises every child cutoff
with fresh recovery. The multi-result factory in `prestart_test.go` verifies
that completed owner inventories keep both returned projections while truncated
ones publish neither. These controls complement the existing analyzer fixtures.


## Post-Start command use and returned handles

One structured command-use query supplies the final detached-intent boundary.
Instruction, possible reachability, closure-binding, operand, reaching-value and
stored-value visits share a candidate child allowance. A completed handoff
witness retains the ordinary missing-wait question; a completed absence retains
unused-command uncertainty. Interrupted discovery has reason
`command-use-budget-exhausted` and never supplies a reportable handoff.
Possible reachability through loop back edges is retained: this structural use
policy differs from a runtime-value use-after census that stops at back edges.
PID/name data and synchronous command-pipe IO remain distinct from handles.

Returned process-owner discovery charges the complete body traversal, including
non-load instructions, and uses `lifecycle.ProveReturnedOwnershipWithin` for
aggregate containment. A possible returned owner remains unknown reaping,
never an exact Wait. Call argument visits also share a child allowance; binding
and completion retain their existing requests. Heap, type and symbol internals
keep independent costs. `command_use_test.go` pins completed positive/negative
SSA shapes, every cutoff, fresh recovery, loop-back-edge semantics and the final
candidate-attributed cutoff trace. Existing fixtures remain the diagnostic and
accepted behavior controls.

## Authoritative pre-Start decision

`proveProcessStart` owns the ordered pre-Start suppression rules. Its structured
state and stable reason drive entry dispatch and the same final trace adapter
used after Start. Caller storage, aggregate elements and possible registration
produce unknown ownership; proven absence of a successful normal return is
accepted. A local wait obligation proceeds to flow analysis and emits no final
pre-Start decision. Existing rule order, query allowances and diagnostics remain.
`prestart_decision_test.go` covers each outcome and exhausted/unavailable inputs;
the analyzer trace test requires one candidate-associated final decision for
caller ownership, deferred registration and a nonreturning success path.


## Successful Start branch allowance

`successfulStartCannotReturn` shares its existing allowance with the exact
success-branch query before asking normal-return reachability. A shortened
error comparison returns an unknown proof with the budget reason; it cannot
become either no successful branch or a proven nonreturning success path.
`startup_queries_test.go` isolates the branch selection cutoff with a live
outer pool and retains the returning, looping and panicking SSA controls plus
intermediate/fresh queries. The pre-Start decision continues to consume that
single returned proof.


## Candidate instruction actions and successful-return proof

`commandProof.action` memoizes the authoritative `processOwnershipAction` by
instruction and exact command SSA value. Classification does not depend on the
flow's current guards, so revisits use the same result rather than repeat
completion/transfer queries and spend another allowance. Primary and merged
commands keep distinct keys. A candidate may retain unknown locally; it never
publishes that cutoff as a complete function summary or shares it with a fresh
candidate.

`proveProcessReturns` owns acquisition-time command resolution, successful
merges, return permissions, immediate process guards and the one post-Start
flow query. Its result carries the final decision, resolved command and
uncovered return to reporting. `reportStartedCommand` presents that result;
pre-Start external-store evidence lives with the prefix proof in `prestart.go`.
The flow still combines primary/merged actions in the original order, preserves
unknown ownership and asks `decideProcessReturn` for the final policy.

`action_cache_test.go` builds an actual deferred Wait and proves cache admission,
revisit after pool exhaustion, distinct-command keys and fresh-candidate
cutoff/recovery. Removing the memo compiles and fails the revisit assertion.
Pinned process controls and complete fixture payloads preserve diagnostics;
final trace decisions agree while repeated evidence queries disappear.

## Immediate guard trace cost

The immediate successful-Start guard still supplies the constant assumption
from its authoritative proof. Its evidence emitter checks the bound probe
before reading positions or formatting the function name, so disabled tracing
creates no metadata. `guard_trace_allocations_test.go` checks that contract,
and `process_guards.go` plus the enabled analyzer trace assert the accepted
guard and its candidate association. Measurements live in
[the performance investigation](../analysis-performance-2026-10-06.md).
