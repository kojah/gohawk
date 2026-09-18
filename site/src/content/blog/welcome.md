---
title: Why I’m building gohawk
description: The resource-management and concurrency bugs I want gohawk to catch, and why trustworthy findings matter more than finding everything.
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

I expected to find something similar already out there. There were tools covering parts of what I wanted, but not quite the combination I was looking for. Go also provides libraries for following values through a program and sharing analysis results across packages. That gave me a starting point for building gohawk.

The Docker example shows why following the program matters. Here's a simplified version of the missing-wait pattern:

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

The fix in Docker handed process management back to Go's standard library. It simplified the code and ensured the child was waited for, including when something went wrong.

A separate lock-order analysis led to a [merged fix in Caddy](https://github.com/caddyserver/caddy/pull/7968), where an error path acquired two locks in the opposite order to another path. Both findings concern relationships between operations that can be some distance apart in the source.

## What else can it catch?

The same kind of oversight shows up beyond child processes. Some of the patterns gohawk checks for are:

- **Resources left open on an error path.** A file gets closed on success, but an early return skips cleanup. Similar mistakes affect HTTP response bodies and database resources.
- **Forgotten cancellation.** A function creates a context, then returns without calling its cancel function or handing that responsibility to its caller.
- **Work left running.** A function starts a goroutine and has a way to wait for it, but one return path skips the wait.
- **Locking mistakes.** A return leaves a mutex locked, or code tries to acquire a lock it already holds. The extended checks also look for conflicting lock orders, as in the Caddy finding.

These are specific checks, not a promise to find every leak or deadlock. The [analyzer catalog](/analyzers/) covers the individual checks and their limits.

## Where gohawk fits

I’m building gohawk to run alongside the tests and analyzers a project already uses. Its focus is the lifetime of resources and concurrent work: who owns something, what needs to happen before it's finished, and whether the code fulfills that responsibility.

## A warning has to earn your attention

Finding a missing cleanup is only part of the problem. Cleanup might happen inside a helper, or responsibility might move to a longer-lived owner.

It would be frustrating if introducing a helper made a warning appear. The tool would effectively be asking you to flatten valid code so it could understand it.

gohawk tries to recognize those safe patterns. Where it can't establish what happened, it may have to stay quiet. Passing a resource to code the analyzer doesn't understand is not, by itself, evidence that the resource was abandoned.

That leaves some bugs undetected. It's a tradeoff I'm willing to make. I want a finding to be worth investigating, without first having to work out whether the tool understood an ordinary ownership handoff.

A useful check therefore needs examples of safe code that resembles the bug, as well as examples where it should report one. That's a central constraint on how I build gohawk.

## What I want from the project

The upstream fixes are a useful measure of progress: a specific finding, a problem worth fixing, and a change that makes the code easier to trust.

That's what I'd like gohawk to contribute to a Go project: another chance to catch the error path you overlooked or the conflicting lock order that was hard to see across functions.

You can find [gohawk on GitHub](https://github.com/kojah/gohawk). If you try it, I'd like to hear about both useful findings and warnings that didn't deserve your attention.
