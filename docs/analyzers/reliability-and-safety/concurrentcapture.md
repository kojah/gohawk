---
title: concurrentcapture
description: "Keep repeatedly launched goroutines from mutating the same local."
---

## Rule details

Goroutines launched from a loop should not mutate the same captured local
without synchronization. Prefer returning results through a channel or keeping
the value local to each goroutine.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6ImNhcHR1cmVkIGxvY2FsIGVyciBpcyBtdXRhdGVkIGJ5IGdvcm91dGluZXMgbGF1bmNoZWQgcmVwZWF0ZWRseSIsInN0YXJ0TGluZSI6NCwic3RhcnRDb2x1bW4iOjYsImVuZExpbmUiOjQsImVuZENvbHVtbiI6MTl9XQ"
func collect(items []int) error {
  var err error
  for range items {
    go func() {
      err = fetch()
    }()
  }
  return err
}
```

### OK

```go
func collectSafely(items []int) error {
  var group sync.WaitGroup
  errs := make(chan error, len(items))
  for range items {
    group.Add(1)
    go func() {
      defer group.Done()
      errs <- fetch()
    }()
  }
  group.Wait()
  close(errs)
  for err := range errs {
    if err != nil {
      return err
    }
  }
  return nil
}
```
<!-- gohawk:generated-examples:end -->
