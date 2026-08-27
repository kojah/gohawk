---
title: apishape
description: "Group error-prone parameters."
---

## Rule details

Group error-prone parameters.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6ImV4cG9ydGVkIEFQSSBoYXMgNSBwYXJhbWV0ZXJzOyB1c2UgYW4gSW5wdXQgb3IgY29uZmlnIHN0cnVjdCIsInN0YXJ0TGluZSI6MCwic3RhcnRDb2x1bW4iOjUsImVuZExpbmUiOjAsImVuZENvbHVtbiI6MTV9LHsibWVzc2FnZSI6IjUgYWRqYWNlbnQgcGFyYW1ldGVycyBzaGFyZSB0eXBlIHN0cmluZzsgdXNlIGFuIElucHV0IHN0cnVjdCIsInN0YXJ0TGluZSI6MCwic3RhcnRDb2x1bW4iOjUsImVuZExpbmUiOjAsImVuZENvbHVtbiI6MTV9XQ"
func CreateUser(name, email, city, country, role string) error {
  return nil
}
```

### OK

```go
type CreateUserInput struct {
  Name, Email, City, Country, Role string
}

func CreateUserWithInput(input CreateUserInput) error { return nil }
```
<!-- gohawk:generated-examples:end -->

## Options

<!-- gohawk:generated-options:start -->
| Knob | Default | Effect |
| --- | --- | --- |
| `max-adjacent-same-type` | `2` | Maximum adjacent parameters of one type; 0 disables the check. |
| `max-parameters` | `4` | Maximum exported function parameters; 0 disables the check. |
<!-- gohawk:generated-options:end -->
