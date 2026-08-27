---
title: syncmapatomicity
description: "Claim sync.Map values atomically."
---

## Rule details

A separate `Load` followed by `Delete` allows another goroutine to observe or
claim the same entry between operations. Use `LoadAndDelete` when the loaded
value is consumed as the successfully removed entry.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

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

### OK

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
