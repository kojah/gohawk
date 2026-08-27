---
title: deferinloop
description: "Release each iteration's resources before starting the next one."
---

## Rule details

A defer inside a loop runs when the surrounding function returns, not when the
iteration ends. Put cleanup-sensitive work in a helper function so files,
locks, timers, and similar resources are released after each iteration.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

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

### OK

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
