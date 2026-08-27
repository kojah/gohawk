---
title: processownership
description: "Wait for started processes."
---

## Rule details

Wait for started processes.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6InN0YXJ0ZWQgY29tbWFuZCBpcyBub3Qgd2FpdGVkIG9uIGV2ZXJ5IHN1Y2Nlc3NmdWwgcmV0dXJuIHBhdGgiLCJzdGFydExpbmUiOjIsInN0YXJ0Q29sdW1uIjo5LCJlbmRMaW5lIjoyLCJlbmRDb2x1bW4iOjI0fV0"
func run(ctx context.Context) error {
  command := exec.CommandContext(ctx, "worker")
  return command.Start()
}
```

### OK

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
