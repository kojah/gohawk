---
title: evalorder
description: "Make dependencies between evaluated operands explicit."
---

## Rule details

Function arguments and return operands are evaluated from left to right. Split
an operation into statements when a later operand mutates a value already read
by an earlier operand.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6ImxhdGVyIG9wZXJhbmQgbWF5IG11dGF0ZSB2YWx1ZSBhZnRlciBpdHMgZWFybGllciB2YWx1ZSB3YXMgZXZhbHVhdGVkIiwic3RhcnRMaW5lIjoxLCJzdGFydENvbHVtbiI6MjQsImVuZExpbmUiOjEsImVuZENvbHVtbiI6MzB9XQ"
func load(value int) (int, error) {
  return value, refresh(&value)
}
```

### OK

```go
func loadInOrder(value int) (int, error) {
  err := refresh(&value)
  return value, err
}
```
<!-- gohawk:generated-examples:end -->
