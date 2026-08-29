---
title: blockingtest
description: "Make test waits cancellable."
---

## What it detects

Reports blocking channel receives and selects in test code when they have no
cancellation or timer escape. It also reports unguarded sends in context-aware
test helpers.

## Why this is flagged

An unconditional wait can leave a test stuck forever when the expected event
never happens. A cancellation or timeout path lets the test stop promptly and
report a useful failure instead of hanging the entire test run.

Further reading: [Go concurrency patterns: Pipelines and cancellation](https://go.dev/blog/pipelines),
[Timing out, moving on](https://go.dev/blog/concurrency-timeouts).

## How to fix it

Wait with a `select` that also listens for the test context or a bounded
timeout. If cancellation wins, stop the test with a message that explains what
it was waiting for.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3sibWVzc2FnZSI6ImJsb2NraW5nIGNoYW5uZWwgcmVjZWl2ZSBpbiB0ZXN0IGNvZGUgcmVxdWlyZXMgY2FuY2VsbGF0aW9uLWF3YXJlIHNlbGVjdCIsInN0YXJ0TGluZSI6MSwic3RhcnRDb2x1bW4iOjksImVuZExpbmUiOjEsImVuZENvbHVtbiI6MTd9XQ"
func waitForEvent(t *testing.T, events <-chan Event) Event {
  return <-events
}
```

### Accepted code

```go
func waitForEventSafely(t *testing.T, events <-chan Event) Event {
  t.Helper()
  select {
  case event := <-events:
    return event
  case <-t.Context().Done():
    t.Fatal("timed out waiting for event")
    return Event{}
  }
}
```
<!-- gohawk:generated-examples:end -->
