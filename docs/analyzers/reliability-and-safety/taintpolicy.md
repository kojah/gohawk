---
title: taintpolicy
description: "Validate untrusted input before a sink."
---

## Rule details

Validate untrusted input before a sink.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged

```go gohawk="W3sibWVzc2FnZSI6InVudHJ1c3RlZCBkYXRhIHJlYWNoZXMgcHJvY2VzcyBzaW5rIGV4ZWMuQ29tbWFuZCIsInN0YXJ0TGluZSI6MSwic3RhcnRDb2x1bW4iOjksImVuZExpbmUiOjEsImVuZENvbHVtbiI6NDB9XQ"
func runConfiguredTool() error {
  return exec.Command(os.Getenv("TOOL")).Run()
}
```

### OK

```go
func runValidatedTool() error {
  tool, err := validateTool(os.Getenv("TOOL"))
  if err != nil {
    return err
  }
  return exec.Command(tool).Run()
}
```
<!-- gohawk:generated-examples:end -->

## Options

<!-- gohawk:generated-options:start -->
| Knob | Default | Effect |
| --- | --- | --- |
| `sanitizers` | empty | Comma-separated fully-qualified sanitizer functions. |
| `sinks` | `filesystem,process,terminal,log` | Comma-separated sink families: filesystem,process,terminal,log. |
<!-- gohawk:generated-options:end -->
