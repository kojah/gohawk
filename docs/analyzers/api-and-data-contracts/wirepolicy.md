---
title: wirepolicy
description: "Tag fields and key literals."
---

## Rule details

Tag fields and key literals.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6InNlcmlhbGl6ZWQgZmllbGQgSUQgcmVxdWlyZXMgYW4gZXhwbGljaXQganNvbiBvciB0b21sIHRhZyIsInN0YXJ0TGluZSI6MSwic3RhcnRDb2x1bW4iOjIsImVuZExpbmUiOjEsImVuZENvbHVtbiI6MTN9LHsibWVzc2FnZSI6InNlcmlhbGl6ZWQgZmllbGQgS2luZCByZXF1aXJlcyBhbiBleHBsaWNpdCBqc29uIG9yIHRvbWwgdGFnIiwic3RhcnRMaW5lIjoyLCJzdGFydENvbHVtbiI6MiwiZW5kTGluZSI6MiwiZW5kQ29sdW1uIjoxM30seyJtZXNzYWdlIjoicGVyc2lzdGVkIG9yIHdpcmUgc3RydWN0IGxpdGVyYWwgbXVzdCB1c2UgZmllbGQga2V5cyIsInN0YXJ0TGluZSI6NSwic3RhcnRDb2x1bW4iOjEyLCJlbmRMaW5lIjo1LCJlbmRDb2x1bW4iOjM3fV0"
type EventRow struct {
  ID   string
  Kind string
}

var event = EventRow{"42", "created"}
```

### OK

```go
type AuditRow struct {
  ID   string `json:"id"`
  Kind string `json:"kind"`
}

var audit = AuditRow{ID: "42", Kind: "created"}
```
<!-- gohawk:generated-examples:end -->
