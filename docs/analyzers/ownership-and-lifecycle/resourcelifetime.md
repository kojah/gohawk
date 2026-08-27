---
title: resourcelifetime
description: "Release owned resources on every path."
---

## Rule details

Release owned resources on every path. Storing a resource in a partially
constructed object does not transfer ownership when an error path returns
without that object.

The built-in contracts cover files, transactions, SQL rows and statements,
HTTP response bodies, timers and tickers, and gzip/zlib readers and writers.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6Im93bmVkIHJlc291cmNlIGZyb20gb3MuT3BlbiBpcyBub3QgcmVsZWFzZWQgb24gZXZlcnkgcmV0dXJuIHBhdGgiLCJzdGFydExpbmUiOjEsInN0YXJ0Q29sdW1uIjoxNSwiZW5kTGluZSI6MSwiZW5kQ29sdW1uIjoyOH1d"
func read(path string) error {
  file, err := os.Open(path)
  if err != nil {
    return err
  }
  _ = file
  return nil
}
```

### OK

```go
func readSafely(path string) error {
  file, err := os.Open(path)
  if err != nil {
    return err
  }
  defer file.Close()
  return nil
}
```
<!-- gohawk:generated-examples:end -->

## Options

<!-- gohawk:generated-options:start -->
| Knob | Default | Effect |
| --- | --- | --- |
| `contracts` | `os,http,sql,time,compress` | Comma-separated resource contract families: os,http,sql,time,compress. |
| `require-reader-close` | `true` | Require gzip and zlib readers to be closed. |
<!-- gohawk:generated-options:end -->
