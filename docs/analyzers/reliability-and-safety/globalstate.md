---
title: globalstate
description: "Give mutable state an owner."
---

## Rule details

Give mutable state an owner.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6Im11dGFibGUgcGFja2FnZSBzdGF0ZSB1c2VycyByZXF1aXJlcyBhbiBpbW11dGFibGUgb3duZXIgb3IgLy9nb2hhd2s6aWdub3JlIGdsb2JhbHN0YXRlIiwic3RhcnRMaW5lIjowLCJzdGFydENvbHVtbiI6NCwiZW5kTGluZSI6MCwiZW5kQ29sdW1uIjoyOX1d"
var users = map[string]User{}
```

### OK

```go
type Store struct {
  users map[string]User
}

func NewStore() *Store {
  return &Store{users: make(map[string]User)}
}
```
<!-- gohawk:generated-examples:end -->

## Options

<!-- gohawk:generated-options:start -->
| Knob | Default | Effect |
| --- | --- | --- |
| `allow-names` | empty | Comma-separated package variable names to allow. |
| `allow-types` | empty | Comma-separated fully-qualified named types to allow. |
<!-- gohawk:generated-options:end -->

Type allowlists use the full import path:

```sh
gohawk -globalstate \
  -globalstate.allow-names=metrics,registry \
  -globalstate.allow-types=example.com/project.Registry \
  ./...
```
