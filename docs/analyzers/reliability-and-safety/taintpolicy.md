---
title: taintpolicy
description: "Validate untrusted input before a sink."
---

## What it detects

Tracks values from environment variables and command-line arguments and reports
when they reach configured filesystem, process, terminal, or logging operations
without passing through a recognized validator or sanitizer.

## Why this is flagged

Input from the environment or another external source may contain paths,
commands, control characters, or other unexpected data. Passing it directly
to a sensitive operation can let that input change what the program does.

Further reading: [OWASP input validation cheat sheet](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html),
[OWASP OS command injection defense](https://cheatsheetseries.owasp.org/cheatsheets/OS_Command_Injection_Defense_Cheat_Sheet.html).

## How to fix it

Validate the value against the small set or format the program actually
accepts before using it. Prefer allowlists and structured APIs; when needed,
pass the value through a trusted sanitizer designed for that specific sink.

## Examples

<!-- gohawk:generated-examples:start -->
### Flagged code

```go gohawk="W3sibWVzc2FnZSI6InVudHJ1c3RlZCBkYXRhIHJlYWNoZXMgcHJvY2VzcyBzaW5rIGV4ZWMuQ29tbWFuZCIsInN0YXJ0TGluZSI6MSwic3RhcnRDb2x1bW4iOjksImVuZExpbmUiOjEsImVuZENvbHVtbiI6NDB9XQ"
func runConfiguredTool() error {
  return exec.Command(os.Getenv("TOOL")).Run()
}
```

### Accepted code

```go
func validateTool(tool string) (string, error) {
  if tool != "compiler" {
    return "", errors.New("unsupported tool")
  }
  return tool, nil
}

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
