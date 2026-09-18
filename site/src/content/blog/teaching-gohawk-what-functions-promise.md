---
title: Teaching gohawk what functions promise
description: How gohawk summarizes cleanup across package boundaries, and why a missing fact cannot prove a bug.
date: 2026-09-18
draft: true
---

<!-- Editorial outline. Explain the fact system built with Go's existing analysis framework, not an invention of the framework itself. -->

## Open with a refactor that should not create a warning

A caller starts a command and waits for it. Move the wait into this helper in another package:

```go
func Reap(command *exec.Cmd) error {
    return command.Wait()
}
```

Now the caller can finish with `return processutil.Reap(command)`. There is no direct `Wait` in the caller, but nothing has gone wrong.

It would be frustrating if introducing that helper made a warning appear. The tool would effectively be asking you to flatten your code so it could understand it.

## Explain the promise the caller needs

Go's analysis framework lets an analyzer export a fact attached to an object. gohawk uses that mechanism to summarize lifecycle behavior.

For this helper, the summary says that its command parameter is waited on before every normal return. The caller applies that summary to the actual command it passed in.

Explain the `Waited` parameter bit with this example before discussing any other fields. Distinguish local evidence from an imported summary.

## Make the promise precise

Contrast the unconditional helper with one that only sometimes waits. The conditional helper cannot earn the same guarantee.

Explain the relevant boundaries: summaries of cleanup describe individual parameters, are computed per function, and concern normal returns. Imported summaries apply to supported directly identified exported callees. Local analysis can still inspect package-internal helpers.

Mention one-package-at-a-time execution under `go vet` only to explain why summaries are useful.

## Preserve uncertainty

Distinguish a summary that does not establish a wait from no summary being available. Neither should casually be translated into proof of a leak.

An absent fact might result from dynamic dispatch or an unavailable summary. An opaque use may therefore suppress a warning.

Contrast guaranteed cleanup or transfer with possible retention: evidence that carries a proof forward must be strict; conservative retention evidence can only hold a diagnostic back.

## End with the design tradeoff

Return to the refactor. The goal is for an analyzer to understand ordinary abstraction without making promises it cannot justify.

Discuss the choice to keep summaries bounded rather than accumulating guesses about framework names. Avoid ending with an inventory of every fact field; link to the reference documentation for that.

## References for developing the draft

- [Go analysis framework](https://pkg.go.dev/golang.org/x/tools/go/analysis)
- [gohawk's fact model](/development/fact-model/)
- [Lifecycle analysis](/architecture/#how-a-lifecycle-analyzer-is-shaped)
