---
title: apishape
description: "Group error-prone parameters."
---

## What it detects

Reports exported functions with too many parameters, long runs of parameters
that share one type, and adjacent optional scalar parameters that are easy to
swap. It also reports types that mix pointer and value receiver methods.

## Why this is flagged

Long parameter lists are difficult to read at call sites, and adjacent values
of the same type can be swapped without the compiler noticing. Grouping related
values in a named type makes each argument's meaning explicit and makes the API
safer to change later.

## How to fix it

Put related parameters into a small, clearly named options or request type.
When two adjacent values could be confused, give them distinct named types or
group them into a type whose field names explain their purpose.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

#### Too many parameters

```go gohawk="W3sibWVzc2FnZSI6ImV4cG9ydGVkIEFQSSBoYXMgNSBwYXJhbWV0ZXJzOyB1c2UgYW4gSW5wdXQgb3IgY29uZmlnIHN0cnVjdCIsInN0YXJ0TGluZSI6MCwic3RhcnRDb2x1bW4iOjUsImVuZExpbmUiOjAsImVuZENvbHVtbiI6MTV9XQ"
func CreateUser(name string, age int, active bool, score float64, role byte) error {
  return nil
}
```

#### Adjacent optional parameters

```go gohawk="W3sibWVzc2FnZSI6ImFkamFjZW50IG9wdGlvbmFsIHNjYWxhciBwYXJhbWV0ZXJzIGFyZSBlYXN5IHRvIHN3YXA7IHVzZSBhbiBJbnB1dCBzdHJ1Y3QiLCJzdGFydExpbmUiOjAsInN0YXJ0Q29sdW1uIjo1LCJlbmRMaW5lIjowLCJlbmRDb2x1bW4iOjEzfV0"
func FindUser(firstName, lastName *string) error {
  return nil
}
```

### Accepted code

```go
type CreateUserInput struct {
  Name, Email, City, Country, Role string
}

func CreateUserWithInput(input CreateUserInput) error {
  return nil
}
```
<!-- gohawk:generated-examples:end -->

## Options

<!-- gohawk:generated-options:start -->
| Knob | Default | Effect |
| --- | --- | --- |
| `max-adjacent-same-type` | `2` | Maximum adjacent parameters of one type; 0 disables the check. |
| `max-parameters` | `4` | Maximum exported function parameters; 0 disables the check. |
<!-- gohawk:generated-options:end -->
