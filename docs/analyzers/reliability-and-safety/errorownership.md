---
title: errorownership
description: "Handle an error at one layer."
---

## What it detects

Handle an error at one layer. The analyzer also catches inline error
declarations whose condition accidentally checks a different error before
returning the newly declared one.

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

```go gohawk="W3sibWVzc2FnZSI6ImVycm9yIGlzIGxvZ2dlZCBhbmQgcmV0dXJuZWQgYnkgc2FtZSBmdW5jdGlvbiIsInN0YXJ0TGluZSI6Miwic3RhcnRDb2x1bW4iOjQsImVuZExpbmUiOjIsImVuZENvbHVtbiI6MTh9XQ"
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
