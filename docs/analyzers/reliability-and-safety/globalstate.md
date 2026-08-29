---
title: globalstate
description: "Give mutable state an owner."
---

## What it detects

Reports mutable package-level variables such as maps, slices, pointers,
interfaces, channels, and function values. Known immutable patterns and
explicitly configured names or types are excluded.

## Why this is flagged

Mutable package state creates hidden dependencies between callers and tests.
It is also easy for goroutines to access it without synchronization, leading
to data races and behavior that depends on execution order.

## How to fix it

Put the state on a type with a clear owner and pass that value to the code that
needs it. If the state must be shared concurrently, keep its synchronization
beside it and expose safe operations instead of the raw value.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3sibWVzc2FnZSI6Im11dGFibGUgcGFja2FnZSBzdGF0ZSB1c2VycyByZXF1aXJlcyBhbiBpbW11dGFibGUgb3duZXIgb3IgLy9nb2hhd2s6aWdub3JlIGdsb2JhbHN0YXRlIiwic3RhcnRMaW5lIjo0LCJzdGFydENvbHVtbiI6NCwiZW5kTGluZSI6NCwiZW5kQ29sdW1uIjoyOX1d"
type User struct {
  Name string
}

var users = map[string]User{}

func rememberUser(id string, user User) {
  users[id] = user
}
```

### Accepted code

```go
type StoredUser struct {
  Name string
}

type Store struct {
  users map[string]StoredUser
}

func NewStore() *Store {
  return &Store{users: make(map[string]StoredUser)}
}

func (store *Store) Remember(id string, user StoredUser) {
  store.users[id] = user
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
