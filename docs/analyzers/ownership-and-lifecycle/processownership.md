---
title: processownership
description: "Wait for started processes."
---

## What it detects

Tracks commands successfully started with `os/exec` and reports return paths
that neither wait for the process nor transfer the command to code that owns
waiting for it.

## Why this is flagged

Starting a process without waiting for it can leave operating-system resources
unreleased and loses the process's final error or exit status. Waiting gives
the child process a complete, observable lifecycle.

Further reading: [`exec.Cmd.Wait`](https://pkg.go.dev/os/exec#Cmd.Wait).

## How to fix it

Use `Run` when the program can wait immediately. If it must use `Start`, make
sure every successful start is followed by `Wait`, and handle the resulting
exit status or error.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3sibWVzc2FnZSI6InN0YXJ0ZWQgY29tbWFuZCBpcyBub3Qgd2FpdGVkIG9uIGV2ZXJ5IHN1Y2Nlc3NmdWwgcmV0dXJuIHBhdGgiLCJzdGFydExpbmUiOjIsInN0YXJ0Q29sdW1uIjo5LCJlbmRMaW5lIjoyLCJlbmRDb2x1bW4iOjI0fV0"
func run(ctx context.Context) error {
  command := exec.CommandContext(ctx, "worker")
  return command.Start()
}
```

### Accepted code

```go
func runSafely(ctx context.Context) error {
  command := exec.CommandContext(ctx, "worker")
  if err := command.Start(); err != nil {
    return err
  }
  return command.Wait()
}
```
<!-- gohawk:generated-examples:end -->
