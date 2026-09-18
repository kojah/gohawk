---
title: Bring Your Own SSA
description: Following one value through a Go function, and seeing what SSA can—and cannot—tell an analyzer.
date: 2026-09-18
draft: true
---

<!-- Editorial outline. Develop this as a standalone tutorial; do not assume the reader has read the gohawk introduction. -->

## Start with a question the source makes awkward

Use one command throughout: after a successful start, does every return path wait for it?

Retain this small example from the original post:

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

The two error returns look much alike. Only the second leaves us responsible for a started process. Searching for a `Wait` call won't help; this function already has one.

## Introduce SSA when we need to follow the paths

Explain static single assignment in plain language: each SSA value has one definition, and uses refer back to it.

Show a small, verified SSA excerpt for the example alongside its control-flow graph. Label any simplified representation explicitly. Follow the successful-start branch to the two later returns.

Keep the focus on value identity and reachable paths. Introduce phi nodes only if an extension of the example needs one; avoid a survey of instruction types.

## Explain what the representation buys us

The analyzer can distinguish a failed start from a successful one and track the particular command involved. A wait on a different command would not satisfy the obligation.

Show one safe variation next to the broken example so the reader can predict how the paths differ.

## Supply the meaning of the calls

SSA doesn't provide the contract of `Start` and `Wait`. The analyzer supplies that: a successful start creates a responsibility, and waiting fulfills it.

Finish with the question of what happens when the wait moves into another function. Link to the fact-system post once it is published, without requiring it to understand this article.

## References for developing the draft

- [Go SSA package](https://pkg.go.dev/golang.org/x/tools/go/ssa)
- [The process-start contract](https://pkg.go.dev/os/exec#Cmd.Start)
- [Reading SSA in gohawk](/development/understanding-ssa/)
