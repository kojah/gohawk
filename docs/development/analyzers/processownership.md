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
