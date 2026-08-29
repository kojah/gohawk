---
title: errorownership
description: "Handle an error at one layer."
---

## What it detects

Handle an error at one layer. The analyzer also catches inline error
declarations whose condition accidentally checks a different error before
returning the newly declared one.

### Checks

<!-- gohawk:generated-checks:start -->
| Check | What it detects | Profile | Tags |
| --- | --- | --- | --- |
| `errorownership/log-and-return` | Reports functions that both log and return the same error. | opt-in | [reliability](../../../tags-and-profiles/#reliability), [policy](../../../tags-and-profiles/#policy) |
| `errorownership/text-classification` | Reports production code that classifies errors by matching their text. | default | [reliability](../../../tags-and-profiles/#reliability) |
| `errorownership/mismatched-inline-error` | Reports inline error declarations whose condition checks a different error. | default | [correctness](../../../tags-and-profiles/#correctness) |
<!-- gohawk:generated-checks:end -->

## Why this is flagged

Handling the same error at several layers often creates duplicate logs or
reports, while checking the wrong variable can silently ignore a real failure.
Each layer should either add context and return the error or handle it fully.

## How to fix it

Choose one responsibility at each layer: return the error with useful context,
or report and fully handle it there. When declaring an error inside an `if`,
make sure the condition checks that newly declared error.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3siY2hlY2siOiJlcnJvcm93bmVyc2hpcC9sb2ctYW5kLXJldHVybiIsIm1lc3NhZ2UiOiJlcnJvciBpcyBsb2dnZWQgYW5kIHJldHVybmVkIGJ5IHNhbWUgZnVuY3Rpb24iLCJzdGFydExpbmUiOjIsInN0YXJ0Q29sdW1uIjo0LCJlbmRMaW5lIjoyLCJlbmRDb2x1bW4iOjE4fV0"
func load() error {
  if err := readConfig(); err != nil {
    log.Print(err)
    return err
  }
  return nil
}
```

### Accepted code

```go
func loadWithContext() error {
  if err := readConfig(); err != nil {
    return fmt.Errorf("read config: %w", err)
  }
  return nil
}
```
<!-- gohawk:generated-examples:end -->
