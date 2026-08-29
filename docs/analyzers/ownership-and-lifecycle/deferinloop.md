---
title: deferinloop
description: "Release each iteration's resources before starting the next one."
---

## What it detects

A defer inside a loop runs when the surrounding function returns, not when the
iteration ends. Put cleanup-sensitive work in a helper function so files,
locks, timers, and similar resources are released after each iteration.

## Why this is flagged

Deferring cleanup until the whole function returns lets resources accumulate
across iterations. A large or long-running loop can then exhaust file handles,
hold locks too long, or keep timers and connections alive unnecessarily.

Further reading: [Effective Go: Defer](https://go.dev/doc/effective_go#defer).

## How to fix it

Move the body of one iteration into a small helper function and defer cleanup
there. The helper returns at the end of each iteration, so its resources are
released before the next iteration starts.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3sibWVzc2FnZSI6ImRlZmVycmVkIGNsZWFudXAgcnVucyBhZnRlciB0aGUgbG9vcCBpbnN0ZWFkIG9mIGFmdGVyIHRoaXMgaXRlcmF0aW9uIiwic3RhcnRMaW5lIjo2LCJzdGFydENvbHVtbiI6NCwiZW5kTGluZSI6NiwiZW5kQ29sdW1uIjoyMn1d"
func readAll(names []string) error {
  for _, name := range names {
    file, err := os.Open(name)
    if err != nil {
      return err
    }
    defer file.Close()
  }
  return nil
}
```

### Accepted code

```go
func readAllSafely(names []string) error {
  for _, name := range names {
    if err := readOne(name); err != nil {
      return err
    }
  }
  return nil
}

func readOne(name string) error {
  file, err := os.Open(name)
  if err != nil {
    return err
  }
  defer file.Close()
  return nil
}
```
<!-- gohawk:generated-examples:end -->
