---
title: concurrentcapture
description: "Keep repeatedly launched goroutines from mutating the same local."
---

## What it detects

Goroutines launched from a loop should not mutate the same captured local
without synchronization. Prefer returning results through a channel or keeping
the value local to each goroutine.

## Why this is flagged

Those goroutines run at unpredictable times and may read or write the captured
value simultaneously. The result is a data race whose output can change from
run to run and may only fail under load.

Further reading: [Data race detector](https://go.dev/doc/articles/race_detector),
[The Go memory model](https://go.dev/ref/mem).

## How to fix it

Keep the changing value local to each goroutine and send the finished result
back through a channel. If the value truly must be shared, protect every access
with the same synchronization mechanism.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

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

### Accepted code

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
