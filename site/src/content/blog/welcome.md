---
title: Why I’m building gohawk
description: How gohawk found bugs in Docker and Caddy, and why I want fewer warnings I can trust.
date: 2026-09-11
draft: true
---

I've been building gohawk, a Go static analysis suite. One of its findings was in Docker: a function started a child process but could return on an error before reaching its `Wait` call, potentially leaving the child unreaped. The [fix was merged](https://github.com/moby/moby/pull/53517).

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

gohawk also checks for files, HTTP response bodies, and database resources left open, along with context cancel functions that never get called or passed back to the caller. For goroutines, it can spot paths that skip an available wait. Other locking checks catch returns that leave a mutex locked or attempts to acquire a lock that's already held.

gohawk runs alongside your existing tests and analyzers. The [analyzer catalog](/analyzers/) explains what each check can catch and where it stops.

## A warning has to earn your attention

Cleanup might happen inside a helper, or another part of the program might take responsibility for it.

Moving cleanup into a helper shouldn't make a warning appear. The tool would be asking you to flatten valid code so it could understand it.

gohawk tries to follow cleanup through those helpers and stays quiet when it can't tell what happens to a resource passed to other code. I'd rather miss some bugs than make you investigate a warning every time the tool can't follow your code.

A check's tests need safe code that looks like the bug, too. Showing that a check finds a missing cleanup isn't enough; it also needs to leave the helper version alone.

You can find [gohawk on GitHub](https://github.com/kojah/gohawk). If you try it, I'd like to hear about both useful findings and warnings that didn't deserve your attention.
