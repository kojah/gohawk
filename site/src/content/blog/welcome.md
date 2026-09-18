---
title: Bring Your Own SSA
description: What building gohawk taught me about static analysis, resource ownership, and deciding when a tool knows enough to report a bug.
date: 2026-09-11
draft: true
---

When I set out to build gohawk a couple of months ago, I expected to find something similar already out there. Go has good tooling for static analysis. Surely someone had used it to catch the resource-management and concurrency bugs I was interested in?

There were tools covering parts of the problem, but I didn't find quite what I was looking for.

That surprised me because Go gives analyzer authors a lot to work with. Beyond the syntax tree, there's an [SSA package](https://pkg.go.dev/golang.org/x/tools/go/ssa) for following values through a function, and an [analysis framework](https://pkg.go.dev/golang.org/x/tools/go/analysis) for sharing findings across packages. You can import these as ordinary Go libraries.

I started building on top of them. What took more work was deciding what the analyzer could safely conclude from the code it saw.

## A brief overview of SSA

SSA stands for *static single assignment*. Each value is defined once, so when you encounter a use of that value, you can trace it back to its definition.

Take a variable assigned in two branches:

```go
var name string
if production {
    name = "prod"
} else {
    name = "dev"
}
fmt.Println(name)
```

In SSA, the two assignments produce separate values. Where the branches meet, a special instruction called a *phi* selects the value coming from the branch that ran.

Control flow is explicit too: the function becomes a graph of blocks, connected by branches. An analyzer can follow a particular value through that graph and ask questions like: “After this file is opened, which paths close it?”

That's useful for the checks I wanted to build. A `Close` somewhere in the function isn't enough. It needs to close the right file, on the paths where that file was successfully opened.

## Surveying the landscape

Go already has tools doing more than inspecting syntax. Staticcheck covers a broad range of mistakes, NilAway focuses on nil-safety, and gosec looks for security problems. Their implementations and goals differ, but they show how much useful analysis is possible beyond matching source patterns.

What I wanted was a suite focused on resource lifetimes and concurrency: processes that aren't waited on, cleanup that happens too early, locks acquired in conflicting orders.

These questions tend to involve several operations, sometimes spread across functions. The interesting part is how those operations relate. Having access to SSA makes that easier to investigate, but it doesn't supply the answer.

## The nature of SSA-backed analysis

Consider a function that starts a child process:

```go
func run(ctx context.Context, wait bool) error {
    command := exec.CommandContext(ctx, "worker")
    if err := command.Start(); err != nil {
        return err
    }
    if wait {
        return command.Wait()
    }
    return nil
}
```

If `Start` succeeds and `wait` is false, we return without waiting for the child. The [`os/exec` contract](https://pkg.go.dev/os/exec#Cmd.Start) requires a successful `Start` to be followed by `Wait` to release the associated resources.

SSA makes the paths easy to distinguish. One returns after a failed start, one calls `Wait`, and one does neither. It also lets us establish that `Start` and `Wait` refer to the same command.

But the analyzer still needs to know the contract. A call to `Start` is just a call until we give it that meaning. So is `Wait`.

In gohawk, the analysis begins by identifying an obligation: this particular command started successfully and needs to be waited on. Later instructions are classified according to what they do with it:

- **Join:** finish the obligation, such as calling `Wait`.
- **Transfer:** hand responsibility to another owner.
- **Unknown:** use the value in a way the analyzer can't interpret.
- **None:** do nothing relevant to the obligation.

The flow analysis then checks whether the obligation is handled on the return paths it examines.

The third category matters a lot. A helper might store the command somewhere and arrange to wait later. If gohawk can't see through that helper, a missing local `Wait` isn't enough evidence to report a bug.

That means accepting some missed bugs. I'm comfortable with that tradeoff: I want a finding to be worth investigating, without first having to work out whether the tool understood an ordinary ownership handoff.

## Prior inspiration

If you've used Xcode, you may have encountered the [Clang Static Analyzer](https://clang.llvm.org/docs/ClangStaticAnalyzer.html). It's an influence on how I think about these checks.

Clang's analyzer uses symbolic execution to explore paths while tracking what it knows about the program. For a memory allocation, that includes whether the memory is still allocated or has been released.

Here's a small example:

```c
char *buf = malloc(1024);
if (buf == NULL) {
    return -1;
}
if (should_abort()) {
    return -1;  // buf leaks
}
free(buf);
return 0;
```

There's a `free` in the function. The problem is the earlier return, reached while `buf` still owns an allocation.

That way of tracking an operation's effect was useful inspiration for gohawk. Its analysis is much more limited than Clang's, and it doesn't perform general symbolic execution. But it still needs a model of what acquiring, releasing, or handing off a resource means.

## Back to Go

Garbage collection removes much of the need to reason about freeing memory. It doesn't wait for a child process, unlock a mutex, or finish a goroutine's work for you.

Those are the kinds of obligations gohawk tracks. And the process example above has a real counterpart in Docker.

Docker's subprocess-based nftables backend started an `nft` command and managed its pipes manually. The final path called `Wait`, but errors while writing input or reading output could return before reaching it.

gohawk's `processownership` analyzer flagged the missing waits. Investigating the code also exposed a deadlock: it read stdout to completion before reading stderr, so a child that filled the stderr pipe could block both processes.

The [merged fix](https://github.com/moby/moby/pull/53517) used `Cmd.Run` with input and output buffers, letting `os/exec` manage the streams and wait for the child. A regression test reproduced the pipe deadlock before the fix and passed afterward.

The missing-wait finding didn't explain every problem in that function. It gave us a concrete reason to inspect it.

## Fact propagation

There is another complication: cleanup often lives in a helper.

Suppose another package exports this function:

```go
func Reap(command *exec.Cmd) error {
    return command.Wait()
}
```

The caller can now finish with `return processutil.Reap(command)`. Its own SSA contains no direct call to `Wait`.

gohawk runs through `go vet`, analyzing one package at a time. Loading every dependency's implementation into every caller would undermine that arrangement. Instead, Go's analysis framework lets an analyzer export a *fact*: a summary attached to an object, which another package can import.

For this helper, gohawk can record that its first parameter is waited on before every normal return. The caller imports that summary and applies it to the command it passed in.

The qualification is essential. If `Reap` only sometimes waits, it doesn't earn that summary. The caller must be able to rely on it without knowing which branch the helper will take.

There's also a difference between an available summary that doesn't prove a wait and having no summary at all. A call through an interface, for example, may leave gohawk without an imported fact for the function that actually runs.

No fact means the analyzer doesn't know. It cannot treat that as proof that cleanup was skipped. This is another place where the tool has to accept a limit on what it can report.

## Conclusion

Building gohawk has involved a lot of decisions like that: which behavior can I establish from the code, and where should the analyzer stop?

SSA and Go's analysis framework take care of substantial infrastructure. The work on top is specific: recognizing a successful process start, following the right value, understanding a helper's cleanup, and checking the relevant paths.

The concurrency checks raise their own questions. A separate lock-order analysis led to a [merged fix in Caddy](https://github.com/caddyserver/caddy/pull/7968), where an error path acquired two locks in the opposite order to another path.

Those fixes are a useful measure of what I'm trying to build. I'd like gohawk to find mistakes that are easy to overlook in review, explain them clearly, and be quiet when it doesn't have enough evidence.

If you're interested in the implementation, the docs cover [the lifecycle analysis](/architecture/#how-a-lifecycle-analyzer-is-shaped), [reading Go's SSA](/development/understanding-ssa/), and [cross-package facts](/development/fact-model/).
