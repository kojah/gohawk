# Editor integration

How gohawk findings reach an editor: the `gohawk lsp` server in
`internal/editor/lsp`, and the VS Code extension in `editors/vscode`. This
note keeps the design, its boundaries, and how to test it.

## Shape

```
VS Code extension (TypeScript, thin)
  |  Language Server Protocol over stdin/stdout
gohawk lsp (long-lived, holds no analysis state)
  |  one child process per check, stopped when outdated or over a limit
go vet -vettool=gohawk -json -overlay=... <package>
  |
go build cache (facts and cached results)
```

The server keeps bookkeeping only: open buffers, a run queue, and the last
findings per package directory. Each check is a `go vet` child with gohawk as
its tool. The child owns all analysis memory and exits when done, so nothing
heavy stays resident beside gopls. A resident whole-program server was
rejected for that reason.

The server never changes which diagnostics exist. It schedules and presents.
It offers no source edits and no action that hides a finding.

## Scheduling

- The edited package is checked first and alone, after a pause in typing
  (750 ms by default; a save skips the pause). Unsaved buffers reach the go
  command through `-overlay`.
- Packages that import it are then checked in one background run, because
  facts flow along imports. The import graph comes from `go list`.
- Any edit cancels the foreground run for that package and the background
  run. Cancelled background work is owed again after the next check.
- A result is applied only if its run is still the active one. A cancelled
  run's result describes old text and is dropped.
- With `run` set to `onSave` or `manual`, findings describe the files as last
  saved: no overlay is used and typing does not cancel a run. A buffer opened
  with text that differs from its file counts as unsaved, and closing an
  unsaved buffer re-checks the package from disk.
- The editor reports files created, changed, or deleted on disk. A deleted
  file or directory loses its findings at once, and a package whose files
  changed outside an open buffer is checked again.
- With a `go.work` file, importers are searched in every module of the
  workspace. A whole-module run covers the module's own packages only, so a
  nested module keeps its findings.
- Only files inside the editor's workspace folders are checked. Editors open
  dependency and standard library files to show a definition; those are not
  the user's code. `_test.go` files and files outside a module are skipped
  with a reason in the status.

## What is never shown as clean

| Situation | Behavior |
|---|---|
| The package does not compile | Earlier findings stay and the package is recorded as unresolved. Compile errors are left to gopls. |
| Some packages of a multi-package run fail to build | Nothing is replaced, because the output does not say which packages were analyzed. Each covered package is then checked on its own. |
| A run passes its time limit | Stopped; its packages are unresolved. |
| A run passes its memory limit | The process tree is killed; its packages are unresolved. |
| A finding names a file outside the packages the run checked | Not filed, and logged. It is never dropped silently. |
| Memory cannot be measured (not Linux) | The soft `GOMEMLIMIT` and the timeout remain. |

An unresolved package stays named in the status until a check of it
completes, however many other packages are checked in between. The status is
derived from what is stored, not from the last run.

Memory is sampled from `/proc` by process group on Linux. The peak reported
for a run is the larger of that sample and the kernel's figure for the
largest single process, so short runs still report a number.

## Presentation

| go vet JSON | Protocol diagnostic |
|---|---|
| `posn`, `end` | `range`, with byte columns converted to UTF-16 using the analyzed text |
| `message` | `message` |
| `category` | `code` |
| catalog documentation URL | `codeDescription` |
| `related` | `relatedInformation`, same order |
| tier | `severity`: core is warning, experimental is information |

Positions from an overlay run name the temporary file. The real path is
encoded into the temporary path itself, because go vet caches output by
content: returning to earlier text replays output that names the temporary
file of an earlier run, or of an earlier server, which no per-run table could
map. A cache hit means the same file names and contents, so the replayed
positions are right.

The go command may report a path with symbolic links resolved. Findings are
stored and published under the editor's spelling of the workspace folder, so
a workspace opened through a link still shows them. `internal/reporting/vetjson` is the one decoder; the terminal
renderer and the server both use it.

The hover adds the catalog's help sentence. Code actions are commands:
step through the path and explain (both implemented by the client, and
offered only when the client says it has them), and open documentation.
Explain runs `gohawk dump trace` for the one candidate and reads saved files.

`gohawk/status` is a custom notification with a state and a composed text,
so every client shows the same wording.

## Settings

`run`, `debounce`, `scope`, `experimental`, `memoryLimit`, `maxParallel`,
`timeout`. They choose when and what to run. None changes what a check
reports; see [analyzers have no options](decisions/2026-09-25-no-analyzer-options.md).
Experimental checks are off by default, as on the command line.

## The extension

The extension resolves the binary in this order: `gohawk.path`, the release
it downloaded into its own storage, then `gohawk` on `PATH`. A download is
the release archive for the platform, checked against `checksums.txt` before
it is unpacked. Each extension version pins one gohawk version.

It adds what the protocol does not provide: the status bar item, path
stepping (decorations built from related locations), and the explanation
tab. Everything the server sends is parsed before use.

## Testing

- `internal/editor/lsp` has protocol tests that drive a real server over
  pipes against a small module, with the built binary as go vet's tool.
- `editors/vscode` has unit tests for its pure modules (`pnpm test`).
- `pnpm acceptance` in `editors/vscode` installs the packaged extension into
  a real VS Code served to a headless browser, plays scripted sessions, and
  writes a report: screenshots, edit-to-findings latency, memory per check,
  crash recovery, and leftover processes.
- `gohawk lsp -event-log FILE` appends one JSON line per scheduling event,
  including each run's peak memory and the server's own memory and goroutine
  count.
  The acceptance run reads it to compute latency; it is also the first thing
  to look at when a finding seems late or missing.

## Measurements

On gohawk itself (61 packages, 2026-11-12, GOMAXPROCS=4):

| Scenario | Wall | Peak memory |
|---|---|---|
| Nothing changed, whole module | 0.55 s | 36 MiB |
| Edit a small package, check it alone | 0.9 s | 83 MiB |
| Edit the largest package (13k lines), check it alone | 2.6 s | 187 MiB |
| Edit a package 39 others import, re-check the module | 10.5 s | 541 MiB |
| Comment-only edit in that package, whole module | 7.3 s | 443 MiB |

A comment-only edit still re-checks importers: the go cache does not cut off
when a package's facts are unchanged.

## Known gaps

- No large-repository measurement yet. A low-level edit with thousands of
  importers may never finish its background run between edits.
- The hard memory limit exists only on Linux.
- Explain shows raw tracer output and ignores unsaved buffers.
- The pinned release must contain the `lsp` command before the extension's
  download path works; until then set `gohawk.path`.
