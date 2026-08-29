---
title: wirepolicy
description: "Tag fields and key literals."
---

## What it detects

Reports exported fields without explicit JSON or TOML tags on types that look
like serialized data. It also reports positional literals for persisted or
wire types when their fields should be named explicitly.

### Checks

<!-- gohawk:generated-checks:start -->
<div class="analyzer-check-list">
  <article class="analyzer-check" id="check-wirepolicy-keyed-literal">
    <code class="analyzer-check-id">wirepolicy/keyed-literal</code>
    <p>Reports positional composite literals for persisted or wire structs.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#reliability">reliability</a>
      <a href="../../../tags-and-profiles/#policy">policy</a>
    </div>
  </article>
  <article class="analyzer-check" id="check-wirepolicy-serialization-tag">
    <code class="analyzer-check-id">wirepolicy/serialization-tag</code>
    <p>Reports exported wire fields without explicit JSON or TOML tags.</p>
    <div class="analyzer-check-tags" aria-label="Tags">
      <a href="../../../tags-and-profiles/#reliability">reliability</a>
      <a href="../../../tags-and-profiles/#policy">policy</a>
    </div>
  </article>
</div>
<!-- gohawk:generated-checks:end -->

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

```go gohawk="W3siY2hlY2siOiJ3aXJlcG9saWN5L3NlcmlhbGl6YXRpb24tdGFnIiwibWVzc2FnZSI6InNlcmlhbGl6ZWQgZmllbGQgSUQgcmVxdWlyZXMgYW4gZXhwbGljaXQganNvbiBvciB0b21sIHRhZyIsInN0YXJ0TGluZSI6MSwic3RhcnRDb2x1bW4iOjIsImVuZExpbmUiOjEsImVuZENvbHVtbiI6MTN9LHsiY2hlY2siOiJ3aXJlcG9saWN5L3NlcmlhbGl6YXRpb24tdGFnIiwibWVzc2FnZSI6InNlcmlhbGl6ZWQgZmllbGQgS2luZCByZXF1aXJlcyBhbiBleHBsaWNpdCBqc29uIG9yIHRvbWwgdGFnIiwic3RhcnRMaW5lIjoyLCJzdGFydENvbHVtbiI6MiwiZW5kTGluZSI6MiwiZW5kQ29sdW1uIjoxM31d"
type EventRow struct {
  ID   string
  Kind string
}
```

#### Positional wire struct literal

```go gohawk="W3siY2hlY2siOiJ3aXJlcG9saWN5L2tleWVkLWxpdGVyYWwiLCJtZXNzYWdlIjoicGVyc2lzdGVkIG9yIHdpcmUgc3RydWN0IGxpdGVyYWwgbXVzdCB1c2UgZmllbGQga2V5cyIsInN0YXJ0TGluZSI6NSwic3RhcnRDb2x1bW4iOjEyLCJlbmRMaW5lIjo1LCJlbmRDb2x1bW4iOjQzfV0"
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
