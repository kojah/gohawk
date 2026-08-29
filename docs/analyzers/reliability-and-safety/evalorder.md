---
title: evalorder
description: "Make dependencies between evaluated operands explicit."
---

## What it detects

Function arguments and return operands are evaluated from left to right. Split
an operation into statements when a later operand mutates a value already read
by an earlier operand.

### Checks

<!-- gohawk:generated-checks:start -->
| Check | What it detects | Profile | Tags |
| --- | --- | --- | --- |
| `evalorder/operand-mutation` | Reports expressions whose later operand mutates a value read by an earlier operand. | default | [correctness](../../../tags-and-profiles/#correctness) |
<!-- gohawk:generated-checks:end -->

## Why this is flagged

Combining a read and a mutation in one expression makes the result depend on a
subtle evaluation-order rule. Separate statements make the intended old and
new values obvious and prevent surprising results during later edits.

Further reading: [The Go specification: Order of evaluation](https://go.dev/ref/spec#Order_of_evaluation).

## How to fix it

Split the expression into separate statements. Save any value that must be read
before the mutation, perform the mutation, and then pass or return the named
results in their intended order.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3siY2hlY2siOiJldmFsb3JkZXIvb3BlcmFuZC1tdXRhdGlvbiIsIm1lc3NhZ2UiOiJsYXRlciBvcGVyYW5kIG1heSBtdXRhdGUgdmFsdWUgYWZ0ZXIgaXRzIGVhcmxpZXIgdmFsdWUgd2FzIGV2YWx1YXRlZCIsInN0YXJ0TGluZSI6Niwic3RhcnRDb2x1bW4iOjI0LCJlbmRMaW5lIjo2LCJlbmRDb2x1bW4iOjMwfV0"
func refresh(value *int) error {
  *value = 42
  return nil
}

func load(value int) (int, error) {
  return value, refresh(&value)
}
```

### Accepted code

```go
func refreshSafely(value *int) error {
  *value = 42
  return nil
}

func loadInOrder(value int) (int, error) {
  err := refreshSafely(&value)
  return value, err
}
```
<!-- gohawk:generated-examples:end -->
