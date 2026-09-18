---
title: Why I’m building gohawk
description: How gohawk found bugs in Docker and Caddy, and why I want fewer warnings I can trust.
date: 2026-09-11
draft: true
---

Docker already waited for the child process. The call was right there at the end of the function.

The problem was the paths that never reached it.

Some error paths returned early, skipping the final `Wait` and potentially leaving the child process unreaped. gohawk, the Go static analysis suite I've been building, flagged those paths. The [fix was merged](https://github.com/moby/moby/pull/53517).

This is the kind of mistake I set out to catch when I started the project. You can read a function, see both the setup and the cleanup, and still miss what happens in between.

## The bugs I was looking for

My interest was resource management and concurrency: a process that nobody waits for, cleanup that runs before work finishes, locks acquired in conflicting orders.

Go's garbage collector doesn't handle those lifetimes for us. Someone still needs to decide when the work is done and who is responsible for finishing it.

Here's a simplified version of the mistake in Docker:

```go
func run(command *exec.Cmd) error {
    if err := command.Start(); err != nil {
        return err
    }
    if err := doSomething(); err != nil {
        return err // The child started, but we never wait.
    }
    return command.Wait()
}
```

The two error returns look much alike. Only the second leaves us responsible for a started process. Searching for a `Wait` call won't help; the function already has one.

The fix wasn't to add a `Wait` before each error return. Docker's code had been managing the child's input and output itself. The change attached those buffers to `exec.Cmd` and used `Run`, letting Go's standard library handle the I/O and wait for the child even if copying failed. That removed the error paths where the wait had been missed.

A separate lock-order check led to a [merged fix in Caddy](https://github.com/caddyserver/caddy/pull/7968), where an error path acquired two locks in the opposite order to another path. Seeing either path alone wouldn't show the conflict.

## What else can it catch?

The same kind of oversight shows up beyond child processes. Some of the patterns gohawk checks for are:

- **Resources left open on an error path.** A file gets closed on success, but an early return skips cleanup. Similar mistakes affect HTTP response bodies and database resources.
- **Forgotten cancellation.** A function creates a context, then returns without calling its cancel function or handing that responsibility to its caller.
- **Work left running.** A function starts a goroutine and has a way to wait for it, but one return path skips the wait.
- **Locking mistakes.** A return leaves a mutex locked, or code tries to acquire a lock it already holds. The extended checks also look for conflicting lock orders, as in the Caddy finding.

gohawk runs alongside your existing tests and analyzers. The [analyzer catalog](/analyzers/) explains what each check can catch and where it stops.

## A warning has to earn your attention

Cleanup might happen inside a helper, or another part of the program might take responsibility for it.

Moving cleanup into a helper shouldn't make a warning appear. The tool would be asking you to flatten valid code so it could understand it.

gohawk tries to follow cleanup through those helpers. If it can't tell what happens to a resource passed to other code, it stays quiet rather than assume the resource was left open.

That means missing some bugs. I'd rather miss those than make you investigate a warning every time the tool can't follow your code.

A check's tests need safe code that looks like the bug, too. Showing that a check finds a missing cleanup isn't enough; it also needs to leave the helper version alone.

You can find [gohawk on GitHub](https://github.com/kojah/gohawk). If you try it, I'd like to hear about both useful findings and warnings that didn't deserve your attention.
