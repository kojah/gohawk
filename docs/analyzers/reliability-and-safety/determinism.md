---
title: determinism
description: "Sort map-derived output."
---

## Rule details

Sort map-derived output.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6Im1hcCBpdGVyYXRpb24gcmVhY2hlcyBvcmRlcmVkIG91dHB1dCB3aXRob3V0IHNvcnRpbmciLCJzdGFydExpbmUiOjIsInN0YXJ0Q29sdW1uIjoyLCJlbmRMaW5lIjoyLCJlbmRDb2x1bW4iOjI1fV0"
func names(users map[string]User) []string {
  var result []string
  for name := range users {
    result = append(result, name)
  }
  return result
}
```

### OK

```go
func sortedNames(users map[string]User) []string {
  var result []string
  for name := range users {
    result = append(result, name)
  }
  slices.Sort(result)
  return result
}
```
<!-- gohawk:generated-examples:end -->
