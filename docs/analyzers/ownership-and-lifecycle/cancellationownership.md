---
title: cancellationownership
description: "Call derived cancel functions."
---

## What it detects

Tracks cancel functions returned by context and signal helpers and reports any
successful return path that neither calls the cancel function nor transfers it
to the caller.

## Why this is flagged

A derived context can retain timers, memory, and references to its parent until
it is canceled. Calling the cancel function promptly releases those resources
and tells dependent work to stop.

Further reading: [Package context](https://pkg.go.dev/context#CancelFunc).

## How to fix it

Keep the cancel function returned when the context is created. Usually, call
`defer cancel()` immediately after checking that creation succeeded, or call it
explicitly at the point where the derived work is finished.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3sibWVzc2FnZSI6ImNhbmNlbCBmdW5jdGlvbiBmcm9tIGNvbnRleHQuV2l0aENhbmNlbCBpcyBub3QgY2FsbGVkIG9uIGV2ZXJ5IHJldHVybiBwYXRoIiwic3RhcnRMaW5lIjoxLCJzdGFydENvbHVtbiI6MTcsImVuZExpbmUiOjEsImVuZENvbHVtbiI6NDN9XQ"
func work(parent context.Context) {
  ctx, cancel := context.WithCancel(parent)
  _ = cancel
  doWork(ctx)
}
```

### Accepted code

```go
func workSafely(parent context.Context) {
  ctx, cancel := context.WithCancel(parent)
  defer cancel()
  doWork(ctx)
}
```
<!-- gohawk:generated-examples:end -->
