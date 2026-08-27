---
title: errorownership
description: "Handle an error at one layer."
---

## Rule details

Handle an error at one layer.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6ImVycm9yIGlzIGxvZ2dlZCBhbmQgcmV0dXJuZWQgYnkgc2FtZSBmdW5jdGlvbiIsInN0YXJ0TGluZSI6Miwic3RhcnRDb2x1bW4iOjQsImVuZExpbmUiOjIsImVuZENvbHVtbiI6MTh9XQ"
func load() error {
  if err := readConfig(); err != nil {
    log.Print(err)
    return err
  }
  return nil
}
```

### OK

```go
func loadWithContext() error {
  if err := readConfig(); err != nil {
    return fmt.Errorf("read config: %w", err)
  }
  return nil
}
```
<!-- gohawk:generated-examples:end -->
