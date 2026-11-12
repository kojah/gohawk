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

| Setting               | Default                | Meaning                                                                           |
| --------------------- | ---------------------- | --------------------------------------------------------------------------------- |
| `gohawk.run`          | `onPause`              | When to check: `onPause`, `onSave`, or `manual`.                                  |
| `gohawk.debounce`     | `750`                  | Milliseconds after the last keystroke.                                            |
| `gohawk.scope`        | `packageThenImporters` | What to check after an edit.                                                      |
| `gohawk.experimental` | `false`                | Include experimental checks.                                                      |
| `gohawk.memoryLimit`  | `2GiB`                 | Memory limit for one check.                                                       |
| `gohawk.maxParallel`  | `2`                    | Packages a background check analyzes at once.                                     |
| `gohawk.timeout`      | `60`                   | Seconds one check may run.                                                        |
| `gohawk.path`         | empty                  | Path to your own gohawk binary. Empty uses the one that ships with the extension. |

Test files (`_test.go`) are not analyzed.

## Development

```sh
pnpm install
pnpm check        # Biome and the TypeScript compiler
pnpm test         # unit tests for the pure modules
pnpm build        # bundle to dist/
pnpm package      # build a .vsix for this platform, with gohawk built from this repository
```

`node scripts/package.mjs --all` builds the six platform packages, each with its own gohawk binary.

Press F5 in this folder to try a change in an Extension Development Host. Set
`gohawk.path` there to a binary built from this repository (`go build -o
gohawk .` at the repository root).

### Acceptance run

```sh
pnpm acceptance --with-go-extension
```

This builds the package for this platform and installs it into a real VS Code served to a headless
browser, plays the scenarios in `acceptance/scenarios.mjs`, and writes
`out/acceptance/report.md` with screenshots, edit-to-findings latency, memory
per check, and anything that looks like a crash or a leftover process. It
needs a Chromium that Playwright can drive; set `ACCEPTANCE_CHROMIUM` to its
path if it is not in Playwright's cache. The process exits nonzero when a
check fails.

On Linux the run fetches Inter and JetBrains Mono once into `.vscode-test/`
and points the browser at them, so screenshots do not use a server's
fallback fonts. Nothing is installed on the machine.

### Stress run

```sh
node acceptance/stress.mjs --gohawk /path/to/gohawk --workspace /path/to/repo --file pkg/big/file.go
```

This drives `gohawk lsp` directly on a real package: it opens the file, makes
a few unsaved edits, and prints for each one how long the check took, its
peak memory, and what the importer run cost. The repository is never
modified.
