---
title: Bring Your Own SSA
description: Building a Go analyzer that finds missing cleanup—and knows when the cleanup might be someone else's job.
date: 2026-09-11
draft: true
---

Docker already waited for the child process. The call was right there at the end of the function.

The problem was the paths that never reached it.

Some error paths returned early, skipping the final `Wait` and potentially leaving the child process unreaped.

This is the kind of mistake I set out to catch when I started building gohawk a couple of months ago. You can read a function, see both the setup and the cleanup, and still miss what happens in between.

My interest was resource management and concurrency: a process that nobody waits for, cleanup that runs before work finishes, locks acquired in conflicting orders. Go's garbage collector doesn't handle those lifetimes for us.

I expected to find something similar already out there. There were tools covering parts of what I wanted, but not quite the combination I was looking for. So I started building on Go's own analysis libraries.

gohawk eventually flagged those missing waits in Docker, and the [fix was merged](https://github.com/moby/moby/pull/53517). But making a check like this useful requires more than recognizing a forgotten wait.

How do you distinguish missing cleanup from cleanup that happens somewhere else?

## Following the path that misses cleanup

Here's a simplified example with the same missing-wait pattern as the Docker finding:

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

The two error returns look much alike. Only the second leaves us responsible for a started process. According to the [`os/exec` contract](https://pkg.go.dev/os/exec#Cmd.Start), a successful `Start` needs a corresponding `Wait` to release the associated resources.

An analyzer has to track that difference. Searching for a `Wait` call won't help; this function already has one.

Go's [SSA package](https://pkg.go.dev/golang.org/x/tools/go/ssa)—short for *static single assignment*—lets us follow a particular value through the different paths a function can take.

Here, that means following the command after a successful start and finding the return that skips its wait.

What SSA doesn't provide is the meaning of `Start` and `Wait`. The analyzer has to supply that: a successful start creates a responsibility, and waiting fulfills it.

If you've used Xcode, you may have encountered the [Clang Static Analyzer](https://clang.llvm.org/docs/ClangStaticAnalyzer.html), which was an influence here. It can track an allocation through different paths and identify a return that leaks it, even when another path correctly frees it.

gohawk doesn't perform Clang's general symbolic execution. But that idea—tracking what an operation makes us responsible for—applies just as well to a child process as it does to allocated memory.

## What if another function handles it?

The example gets less obvious when we move the wait into a helper:

```go
func Reap(command *exec.Cmd) error {
    return command.Wait()
}
```

Now the caller can finish with `return processutil.Reap(command)`. There is no direct `Wait` in the caller, but nothing has gone wrong.

It would be frustrating if introducing that helper made a warning appear. The tool would effectively be asking you to flatten your code so it could understand it.

Go's [analysis framework](https://pkg.go.dev/golang.org/x/tools/go/analysis) lets gohawk share a summary between packages: this helper waits for the command it's given. The caller can rely on that summary without inspecting the helper's implementation again.

The summary has to be trustworthy, though. A helper that only sometimes calls `Wait` doesn't earn an unconditional promise that it waits. Otherwise, moving a bug into a helper would make it disappear from the analysis.

## Where the evidence runs out

A helper that directly calls `Wait` is the easy case.

What about one that stores the command in a manager? Or passes it to a callback? Perhaps another goroutine will wait for it after the current function returns.

Sometimes gohawk can establish that responsibility has moved elsewhere. Sometimes it can't. Those cases need different treatment from a return that simply abandons the command.

The lifecycle analysis therefore makes room for three meaningful outcomes: cleanup happened, responsibility was transferred, or the use is unknown. Instructions unrelated to the obligation can be ignored.

If a command is passed to code the analyzer can't interpret, gohawk may have to suppress the warning. It cannot assume that an operation did nothing just because it couldn't explain it.

That deliberately leaves some bugs undetected. It also avoids asking users to reorganize valid code, add suppressions, or investigate warnings caused by the tool's own blind spots.

This is a central constraint on how I build gohawk. A useful check needs a clear account of the safe cases that resemble the bug. Getting the small example to produce a warning is only part of the job.

## Back to Docker

The Docker fix handed process management back to Go's standard library. That simplified the code and ensured that the child was waited for, including when something went wrong.

That's the sort of result I want from gohawk: a specific finding that gives you a reason to look closely, followed by a fix that makes the code easier to trust.

A separate lock-order analysis also led to a [merged fix in Caddy](https://github.com/caddyserver/caddy/pull/7968), where an error path acquired two locks in the opposite order to another path. There are more kinds of mistakes to look for, each with its own questions about what the analyzer can prove.

For now, the standard I want to hold the project to is fairly practical. If gohawk flags something, you should be able to understand why it thinks there's a bug. And if you hand cleanup to another part of your program, it should have a better reason to complain than “I didn't see a close.”

You can find [gohawk on GitHub](https://github.com/kojah/gohawk). The docs go further into [lifecycle analysis](/architecture/#how-a-lifecycle-analyzer-is-shaped) and [cross-package facts](/development/fact-model/) if you're interested in how the checks work.
