---
title: exitpolicy
description: "Do not bypass registered cleanup when terminating the process."
---

## What it detects

`os.Exit` and `log.Fatal` terminate the process without running deferred calls.
Return an error through the normal call stack when cleanup has already been
registered on the current path.

### Checks

<!-- gohawk:generated-checks:start -->
<div class="analyzer-check-list">
  <article class="analyzer-check" id="check-exitpolicy-skipped-defer">
    <code class="analyzer-check-id">exitpolicy/skipped-defer</code>
    <p>Reports immediate process termination that bypasses an earlier defer.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#correctness">correctness</a>
    </div>
  </article>
</div>
<!-- gohawk:generated-checks:end -->

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

```go gohawk="W3siY2hlY2siOiJleGl0cG9saWN5L3NraXBwZWQtZGVmZXIiLCJtZXNzYWdlIjoibG9nLkZhdGFsIGV4aXRzIHdpdGhvdXQgcnVubmluZyBhbiBlYXJsaWVyIGRlZmVyIiwic3RhcnRMaW5lIjozLCJzdGFydENvbHVtbiI6MiwiZW5kTGluZSI6MywiZW5kQ29sdW1uIjoyOX1d"
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
