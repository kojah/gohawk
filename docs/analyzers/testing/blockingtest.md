---
title: blockingtest
description: "Make test waits cancellable."
---

## Rule details

Make test waits cancellable.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6ImJsb2NraW5nIGNoYW5uZWwgcmVjZWl2ZSBpbiB0ZXN0IGNvZGUgcmVxdWlyZXMgY2FuY2VsbGF0aW9uLWF3YXJlIHNlbGVjdCIsInN0YXJ0TGluZSI6MSwic3RhcnRDb2x1bW4iOjksImVuZExpbmUiOjEsImVuZENvbHVtbiI6MTd9XQ"
func waitForEvent(t *testing.T, events <-chan Event) Event {
  return <-events
}
```

### OK

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
