---
title: wirepolicy
description: "Tag fields and key literals."
---

## What it detects

Reports exported fields without explicit JSON or TOML tags on types that look
like serialized data. It also reports positional literals for persisted or
wire types when their fields should be named explicitly.

## Why this is flagged

Serialized data is a contract with other programs and stored data. Explicit
field tags keep that contract stable when Go names change, while keyed literals
keep construction correct when fields are added or reordered.

## How to fix it

Add the appropriate serialization tag to every exported wire field. When
constructing a wire value, name each field in the literal instead of relying on
the fields' current order.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

#### Missing serialization tags

```go gohawk="W3sibWVzc2FnZSI6InNlcmlhbGl6ZWQgZmllbGQgSUQgcmVxdWlyZXMgYW4gZXhwbGljaXQganNvbiBvciB0b21sIHRhZyIsInN0YXJ0TGluZSI6MSwic3RhcnRDb2x1bW4iOjIsImVuZExpbmUiOjEsImVuZENvbHVtbiI6MTN9LHsibWVzc2FnZSI6InNlcmlhbGl6ZWQgZmllbGQgS2luZCByZXF1aXJlcyBhbiBleHBsaWNpdCBqc29uIG9yIHRvbWwgdGFnIiwic3RhcnRMaW5lIjoyLCJzdGFydENvbHVtbiI6MiwiZW5kTGluZSI6MiwiZW5kQ29sdW1uIjoxM31d"
type EventRow struct {
  ID   string
  Kind string
}
```

#### Positional wire struct literal

```go gohawk="W3sibWVzc2FnZSI6InBlcnNpc3RlZCBvciB3aXJlIHN0cnVjdCBsaXRlcmFsIG11c3QgdXNlIGZpZWxkIGtleXMiLCJzdGFydExpbmUiOjUsInN0YXJ0Q29sdW1uIjoxMiwiZW5kTGluZSI6NSwiZW5kQ29sdW1uIjo0M31d"
type TaggedEventRow struct {
  ID   string `json:"id"`
  Kind string `json:"kind"`
}

var event = TaggedEventRow{"42", "created"}
```

### Accepted code

```go
type AuditRow struct {
  ID   string `json:"id"`
  Kind string `json:"kind"`
}

var audit = AuditRow{ID: "42", Kind: "created"}
```
<!-- gohawk:generated-examples:end -->
