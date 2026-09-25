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

A command captured by a callback passed to an opaque runner is an uncertain
handoff, not a proven leak or a guaranteed wait. A launched waiter with an
explicit `Wait` followed by process termination is also outside the normal-return
completion proof. Locally stored command fields are resolved at acquisition when
checking whether a returned value owner contains the command.

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
