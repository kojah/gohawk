# gohawk for VS Code

Shows [gohawk](https://gohawk.dev) findings in the editor: resources that are
not released on every return path, contradictory lock orders, goroutines
nothing waits for, and more. Each finding carries the path that proves it.

- Findings appear shortly after you stop typing, including in unsaved files.
- Hover a finding for the usual fix; use the lightbulb to step through the
  reported path or to see the full evidence.
- gohawk only reports. It never edits your code.

It runs beside the official Go extension and does not replace it.

## How it works

The extension starts `gohawk lsp`, a small process that holds no analysis
state. Each check runs `go vet` with gohawk as its tool and exits when done,
so analysis memory is released between checks.

## Settings

| Setting               | Default                | Meaning                                          |
| --------------------- | ---------------------- | ------------------------------------------------ |
| `gohawk.run`          | `onPause`              | When to check: `onPause`, `onSave`, or `manual`. |
| `gohawk.debounce`     | `750`                  | Milliseconds after the last keystroke.           |
| `gohawk.scope`        | `packageThenImporters` | What to check after an edit.                     |
| `gohawk.experimental` | `false`                | Include experimental checks.                     |
| `gohawk.memoryLimit`  | `2GiB`                 | Memory limit for one check.                      |
| `gohawk.maxParallel`  | `2`                    | Packages a background check analyzes at once.    |
| `gohawk.timeout`      | `60`                   | Seconds one check may run.                       |
| `gohawk.path`         | empty                  | Path to a gohawk binary.                         |

Test files (`_test.go`) are not analyzed.

## Development

```sh
pnpm install
pnpm check      # Biome and the TypeScript compiler
pnpm test       # unit tests
pnpm build      # bundle to dist/
pnpm package    # build a .vsix
```
