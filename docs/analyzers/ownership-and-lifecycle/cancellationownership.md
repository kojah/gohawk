---
title: cancellationownership
description: "Call derived cancel functions."
---

## Rule details

Call derived cancel functions.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6ImNhbmNlbCBmdW5jdGlvbiBmcm9tIGNvbnRleHQuV2l0aENhbmNlbCBpcyBub3QgY2FsbGVkIG9uIGV2ZXJ5IHJldHVybiBwYXRoIiwic3RhcnRMaW5lIjoxLCJzdGFydENvbHVtbiI6MTcsImVuZExpbmUiOjEsImVuZENvbHVtbiI6NDN9XQ"
func work(parent context.Context) {
  ctx, cancel := context.WithCancel(parent)
  _ = cancel
  doWork(ctx)
}
```

### OK

```go
func workSafely(parent context.Context) {
  ctx, cancel := context.WithCancel(parent)
  defer cancel()
  doWork(ctx)
}
```
<!-- gohawk:generated-examples:end -->
