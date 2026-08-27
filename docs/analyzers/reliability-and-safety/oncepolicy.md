---
title: oncepolicy
description: "Keep sync.Once function wrappers alive across calls."
---

## Rule details

`sync.OnceFunc`, `sync.OnceValue`, and `sync.OnceValues` preserve their state in
the function value they return. Store that wrapper instead of constructing,
calling, and discarding it in one expression.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6InN5bmMuT25jZUZ1bmMgd3JhcHBlciBpcyBkaXNjYXJkZWQgYWZ0ZXIgb25lIGNhbGwiLCJzdGFydExpbmUiOjEsInN0YXJ0Q29sdW1uIjoyLCJlbmRMaW5lIjoxLCJlbmRDb2x1bW4iOjI5fV0"
func start() {
  sync.OnceFunc(initialize)()
}
```

### OK

```go
func startOnce() {
  initializeOnce()
}
```
<!-- gohawk:generated-examples:end -->
