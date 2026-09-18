---
title: Bring Your Own SSA
description: Building a Go analyzer that finds missing cleanup—and knows when the cleanup might be someone else's job.
date: 2026-09-11
draft: true
---

Docker already waited for the child process. The call was right there at the end of the function.

The problem was the paths that never reached it.

In Docker's subprocess-based nftables backend, errors while writing input or reading output could return before the final `Wait`. Once the child had started, those returns could leave it unreaped.

gohawk, the Go static analysis suite I've been building, flagged the missing waits. The [fix eventually merged](https://github.com/moby/moby/pull/53517), along with a regression test for a related pipe deadlock uncovered while investigating the code.

It's a useful example of the kind of bug I wanted to catch. You can read the function, see both the setup and the cleanup, and still miss what happens in between.

But building a tool to catch it raises another question: how do you distinguish missing cleanup from cleanup that happens somewhere else?

## Why build another Go analyzer?

When I set out to build gohawk a couple of months ago, I expected to find something similar already out there.

Go has good tooling for static analysis. Beyond its syntax tree, there's an [SSA package](https://pkg.go.dev/golang.org/x/tools/go/ssa) for following values through functions, and an [analysis framework](https://pkg.go.dev/golang.org/x/tools/go/analysis) for carrying findings across packages. You can import these as ordinary Go libraries.

There are established tools using that infrastructure. Staticcheck covers a broad range of mistakes, NilAway focuses on nil-safety, and gosec looks for security problems. I found tools covering parts of what I wanted, but not quite the combination I was looking for.

My interest was resource management and concurrency: a process that nobody waits for, cleanup that runs before work finishes, locks acquired in conflicting orders.

Go's garbage collector doesn't handle those lifetimes for us. Someone still has to decide when the work is done and who is responsible for finishing it.

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

SSA—*static single assignment*—gives us a convenient representation for asking more precise questions. Each value is defined once, and the function's control flow is represented as a graph of blocks.

That lets the analyzer follow the particular command we started, distinguish the success and failure branches, and inspect the returns reachable after success. It doesn't have to infer the structure from how the source happens to be written.

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

Go's analysis framework provides a way to avoid that. An analyzer can summarize something it established about a function and export the summary as a *fact*. A caller in another package can import that fact without inspecting the helper's body.

For `Reap`, gohawk can record that the command parameter is waited on before every normal return. The caller applies that summary to the command it passed in.

This also fits how gohawk runs: through `go vet`, one package at a time. Dependencies can carry small summaries instead of requiring the analyzer to load the entire program.

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

In the Docker finding, the early error paths returned without waiting for the started command. Investigating those paths also exposed a separate problem with the pipe handling.

The code drained stdout before reading stderr. If the child filled the stderr pipe while stdout remained open, the child could block writing stderr while Docker waited for stdout to finish.

The missing-wait diagnostic led us to that code; it didn't itself diagnose the pipe deadlock.

The fix simplified the whole operation. It attached input and output buffers to the command and used `Cmd.Run`, letting `os/exec` handle the streams and wait for the process. A regression test reproduced the deadlock before the change and passed afterward.

That's the sort of result I want from gohawk: a specific finding that gives you a reason to look closely, followed by a fix that makes the code easier to trust.

A separate lock-order analysis also led to a [merged fix in Caddy](https://github.com/caddyserver/caddy/pull/7968), where an error path acquired two locks in the opposite order to another path. There are more kinds of mistakes to look for, each with its own questions about what the analyzer can prove.

For now, the standard I want to hold the project to is fairly practical. If gohawk flags something, you should be able to understand why it thinks there's a bug. And if you hand cleanup to another part of your program, it should have a better reason to complain than “I didn't see a close.”

You can find [gohawk on GitHub](https://github.com/kojah/gohawk). The docs go further into [lifecycle analysis](/architecture/#how-a-lifecycle-analyzer-is-shaped) and [cross-package facts](/development/fact-model/) if you're interested in how the checks work.
