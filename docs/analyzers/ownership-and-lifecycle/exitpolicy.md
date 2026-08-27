---
title: exitpolicy
description: "Do not bypass registered cleanup when terminating the process."
---

## Rule details

`os.Exit` and `log.Fatal` terminate the process without running deferred calls.
Return an error through the normal call stack when cleanup has already been
registered on the current path.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6ImxvZy5GYXRhbCBleGl0cyB3aXRob3V0IHJ1bm5pbmcgYW4gZWFybGllciBkZWZlciIsInN0YXJ0TGluZSI6Mywic3RhcnRDb2x1bW4iOjIsImVuZExpbmUiOjMsImVuZENvbHVtbiI6Mjl9XQ"
func run() {
  file, _ := os.CreateTemp("", "state")
  defer file.Close()
  log.Fatal("startup failed")
}
```

### OK

```go
func runSafely() error {
  file, err := os.CreateTemp("", "state")
  if err != nil {
    return err
  }
  defer file.Close()
  return nil
}
```
<!-- gohawk:generated-examples:end -->
