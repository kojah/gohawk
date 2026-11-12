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
- A package with more importers than `importerLimit` (50 by default) is
  followed only into the importers that have a file open. The status says
  how many were not re-checked until a whole-module check covers them. On
  Kubernetes, the importers of `pkg/apis/core` take about four minutes to
  re-check; without the limit that would start after every pause.
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
| A run goes longer than the time limit without finishing a package | Stopped; its packages are unresolved. |
| A run passes its memory limit | The process tree is killed; its packages are unresolved. |
| A finding names a file outside the packages the run checked | Not filed, and logged. It is never dropped silently. |
| Memory cannot be measured (not Linux) | The soft `GOMEMLIMIT` and the timeout remain. |

An unresolved package stays named in the status until a check of it
completes, however many other packages are checked in between. The status is
derived from what is stored, not from the last run.

The time limit is an idle limit, and progress has two sources. The go command
is run with `-v`, which makes it name each package it compiles. And gohawk,
running as go vet's tool, appends a line to the run's progress file for each
package it starts analyzing (the server passes the file's path in the
environment). Either one resets the clock, and a run is stopped when it goes
`timeout` seconds without any. The limit therefore bounds one package's work
and not the run.

This matters on a large repository: the first check must compile and analyze
every dependency once. A limit on the whole run reported that as "stopped"
again and again. The compile names alone were not enough either: once
compiling finished, a run that was busy analyzing dependencies looked idle.
While a run is active the status shows the packages analyzed so far and the
elapsed time. A run is also stopped after 30 minutes however steadily it
progresses.

Memory is sampled from `/proc` by process group on Linux. The peak reported
for a run is the larger of that sample and the kernel's figure for the
largest single process, so short runs still report a number.

## Limits on the server itself

The limits above bound a check. These bound the server and what it leaves
behind:

- Only unsaved Go buffers inside the workspace are written to an overlay,
  and a cancelled run stops writing at once.
- The package listing runs under the same deadline and memory watch as the
  analysis. Output from a command is capped, and so is a header line.
- One explanation runs at a time, under a check's limits.
- On exit, end of input, interrupt, or terminate, the server stops every
  check and waits for the processes to be gone before it returns.
- A server that is killed outright cannot do that. Each process a check
  starts carries the server's process ID, and the analysis tool stops itself
  when that server no longer exists. A starting server removes the
  temporary directories of servers that are gone.
- A message that is not valid costs one error reply; the server reads on. A
  header with no usable length loses the stream and ends the server.
- Messages to the client are queued for one writer. A client that stops
  reading is declared stalled after a bounded wait and the server ends,
  instead of hanging where no exit request can reach it.
- Nothing is checked before `initialize`, because the workspace is unknown.
- Setting values far outside what a machine can do are bounded or replaced
  by defaults.

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

`run`, `debounce`, `scope`, `importerLimit`, `experimental`, `memoryLimit`,
`maxParallel`, `timeout`. They choose when and what to run. None changes what a check
reports; see [analyzers have no options](decisions/2026-09-25-no-analyzer-options.md).
Experimental checks are off by default, as on the command line.

## The extension

Each extension package carries the gohawk binary for its platform in `bin/`.
The Marketplace serves the package that matches the user's platform, so
nothing is downloaded at run time and the binary always matches the
extension. The extension resolves the binary in this order: `gohawk.path`,
the bundled binary, then `gohawk` on `PATH`. A configured path that does not
work is reported, not silently replaced.

`node scripts/package.mjs` builds a package: for the host platform by
default, for one target with `--target`, or for all six with `--all`. Without
`--binary` it cross-compiles gohawk from the repository. The release workflow
packages each platform from the release binaries with the release version,
and attaches the `.vsix` files to the GitHub release. Publishing them to the
Marketplace is a manual step: `vsce publish --packagePath <file>` for each.

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

## Adversarial testing

Three agents attacked the first version (2026-11-12): one for memory and
latency, one for wrong or stale results against the command-line result, and
one for crashes and protocol abuse. They found 25 defects and no panics; the
regression scenarios in `truthfulness_test.go` and `protocol_test.go` come
from them. Worth repeating after any change to scheduling, the store, or the
transport: the oracle is that what the editor shows must equal what
`gohawk -json` reports for the same content.

## Known gaps

- Large packages are not live: re-checking Kubernetes' `pkg/kubelet` (38k
  lines) takes 9 to 16 seconds at about 1.1 GiB, and even an answer from the
  cache costs 6 to 7 seconds there, which is the go command loading the
  dependency graph.
- The hard memory limit exists only on Linux.
- Explain shows raw tracer output and ignores unsaved buffers.
- A package with thousands of findings produces one large publish message;
  nothing caps findings per file.
- A generated file of a few thousand lines can take longer than the default
  time limit. It is then reported as stopped, not analyzed faster.
- Every gohawk release needs a matching extension release, because the
  binary ships inside the extension.
