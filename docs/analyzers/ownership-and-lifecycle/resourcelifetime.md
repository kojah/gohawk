---
title: resourcelifetime
description: "Release owned resources on every path."
---

## What it detects

Release owned resources on every path. Storing a resource in a partially
constructed object does not transfer ownership when an error path returns
without that object.

The built-in contracts cover files, transactions, SQL rows and statements,
HTTP response bodies, timers and tickers, and gzip/zlib readers and writers.

## Why this is flagged

Owned resources are limited and often hold work open elsewhere. Missing cleanup
on even one return path can leak file descriptors, database connections,
network bodies, timers, or transactions until the program slows down or fails.

Further reading: [Effective Go: Defer](https://go.dev/doc/effective_go#defer),
[Package net/http](https://pkg.go.dev/net/http#Response).

## How to fix it

As soon as a resource is acquired successfully, arrange for its matching
cleanup on every later return path. Use `defer` when the current function owns
the resource, or transfer it only when the receiver clearly takes ownership.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

#### Leaked file

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

#### Leaked database rows

```go gohawk="W3sibWVzc2FnZSI6Im93bmVkIHJlc291cmNlIGZyb20gc3FsLlF1ZXJ5Q29udGV4dCBpcyBub3QgcmVsZWFzZWQgb24gZXZlcnkgcmV0dXJuIHBhdGgiLCJzdGFydExpbmUiOjEsInN0YXJ0Q29sdW1uIjoxNSwiZW5kTGluZSI6MSwiZW5kQ29sdW1uIjo1M31d"
func query(ctx context.Context, database *sql.DB) error {
  rows, err := database.QueryContext(ctx, "SELECT 1")
  if err != nil {
    return err
  }
  _ = rows
  return nil
}
```

### Accepted code

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
