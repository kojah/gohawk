---
title: exitpolicy
description: "Do not bypass registered cleanup when terminating the process."
---

## What it detects

`os.Exit` and `log.Fatal` terminate the process without running deferred calls.
Return an error through the normal call stack when cleanup has already been
registered on the current path.

## Why this is flagged

Immediate process termination skips deferred cleanup. Buffered output may be
lost, temporary files may remain, and resources may not be closed cleanly.

Further reading: [`os.Exit`](https://pkg.go.dev/os#Exit),
[`log.Fatal`](https://pkg.go.dev/log#Fatal).

## How to fix it

Return an error through the normal call stack so deferred cleanup can run. If
the process must exit, do it once at the top level, after the function that owns
the cleanup has returned.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3sibWVzc2FnZSI6ImxvZy5GYXRhbCBleGl0cyB3aXRob3V0IHJ1bm5pbmcgYW4gZWFybGllciBkZWZlciIsInN0YXJ0TGluZSI6Mywic3RhcnRDb2x1bW4iOjIsImVuZExpbmUiOjMsImVuZENvbHVtbiI6Mjl9XQ"
func run() {
  file, _ := os.CreateTemp("", "state")
  defer file.Close()
  log.Fatal("startup failed")
}
```

### Accepted code

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
