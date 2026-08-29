---
title: syncmapatomicity
description: "Claim sync.Map values atomically."
---

## What it detects

A separate `Load` followed by `Delete` allows another goroutine to observe or
claim the same entry between operations. Use `LoadAndDelete` when the loaded
value is consumed as the successfully removed entry.

## Why this is flagged

Another goroutine can change the map between two separate operations. The
caller may then process a value it did not actually remove, allowing the same
work to be claimed twice or a newer value to be deleted accidentally.

Further reading: [`sync.Map.LoadAndDelete`](https://pkg.go.dev/sync#Map.LoadAndDelete).

## How to fix it

Replace the separate `Load` and `Delete` with one `LoadAndDelete` operation.
Only process the returned value when that atomic operation reports that the
entry was present.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3sibWVzc2FnZSI6InN5bmMuTWFwIExvYWQgYW5kIERlbGV0ZSBkbyBub3QgYXRvbWljYWxseSBjbGFpbSB0aGUgdmFsdWUiLCJzdGFydExpbmUiOjMsInN0YXJ0Q29sdW1uIjo0LCJlbmRMaW5lIjozLCJlbmRDb2x1bW4iOjIxfV0"
func take(cache *sync.Map, key string) any {
  value, ok := cache.Load(key)
  if ok {
    cache.Delete(key)
    return value
  }
  return nil
}
```

### Accepted code

```go
func takeAtomically(cache *sync.Map, key string) any {
  value, deleted := cache.LoadAndDelete(key)
  if deleted {
    return value
  }
  return nil
}
```
<!-- gohawk:generated-examples:end -->
