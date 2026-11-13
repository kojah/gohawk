---
title: VS Code
description: See gohawk findings in VS Code while you edit.
---

The gohawk extension shows findings in the editor a moment after you stop
typing, with the steps that lead to each one. It runs beside the official Go
extension and does not replace it.

![A gohawk finding in VS Code, with its steps and the usual fix](/editor/finding.png)

## Install

Search for **gohawk** in the Extensions view and install it. Cursor and other
editors that use Open VSX can install it the same way. The extension includes
the gohawk binary for your platform, so there is nothing else to download. It needs a supported Go command on your `PATH`, the same as the CLI.

Open a Go file in a module. The status bar shows `gohawk` and the result of
the last check.

## Read a finding

A finding is underlined where the problem starts, such as the call that opens
a file. Hover it to see:

- the message and the check that reported it;
- the steps that lead to the problem, each a link to its line;
- the usual fix.

The Problems panel lists the same steps under each finding.

## Follow the path

Put the cursor on a finding and open the lightbulb menu (`Ctrl+.`, or `Cmd+.`
on a Mac). **Step through the reported path** dims everything that is not part
of the path and numbers each step. `Alt+]` and `Alt+[` move between steps and
`Esc` exits. A step in another file opens that file.

![Stepping through the path of a finding](/editor/path.png)

gohawk reports problems. It does not change your code, so the menu has no
automatic fix.

## When checks run

gohawk checks the package you are editing about a second after you stop
typing. It reads your unsaved changes, so you do not need to save first. It
then checks the packages that import that package, because a change in one
package can add or remove findings in another.

The status bar tells you what happened:

| Status | Meaning |
|---|---|
| `no findings` | The last check found nothing. |
| `2 findings` | Findings are listed in the Problems panel. |
| `checking server… 412 packages, 1m20s` | A check is running. |
| `build failed in server` | The package does not compile, so it cannot be checked. Earlier findings stay until it compiles again. |
| `stopped at a limit: server` | A check used too much time or memory and was stopped. |
| `12 importers not re-checked` | The package has many importers. Only the ones you have open were checked. |
| `not checked` | The file is a test file, or is outside your workspace or module. |

Run **gohawk: Show Output** for a log of each check.

## Large projects

The first check in a project also analyzes its dependencies. On a very large
project this takes a few minutes, once. The status bar shows the progress, and
later checks reuse the work.

A large package takes longer to check than a small one. If checks feel slow
while you type, set `gohawk.run` to `onSave`.

## Settings

| Setting | Default | Meaning |
|---|---|---|
| `gohawk.run` | `onPause` | Check after you stop typing, `onSave`, or only on request (`manual`). |
| `gohawk.debounce` | `750` | How long to wait after the last keystroke, in milliseconds. |
| `gohawk.scope` | `packageThenImporters` | Also check importers, only the edited `package`, or the whole module on save (`workspaceOnSave`). |
| `gohawk.importerLimit` | `50` | With more importers than this, check only the ones you have open. |
| `gohawk.experimental` | `false` | Include [experimental checks](/configuration/#choose-what-runs). |
| `gohawk.memoryLimit` | `2GiB` | Stop a check that uses more memory than this. |
| `gohawk.maxParallel` | `2` | Packages analyzed at once. |
| `gohawk.timeout` | `60` | Stop a check that spends this many seconds on one package. |
| `gohawk.path` | empty | Use your own gohawk binary. |

## What is not checked

- Test files (`_test.go`).
- Files outside the folder you opened, such as a dependency you jumped into.
- Experimental checks, unless you turn them on.

## Other editors

The extension is a thin client for `gohawk lsp`, which speaks the Language
Server Protocol on standard input and output. Any editor with a language
client can start it for Go files and will show the same findings with their
steps. Path stepping and the status bar are specific to VS Code.

## If something looks wrong

- **`gohawk: not installed`.** The extension found no gohawk binary it can
  run. Reinstall the extension, or set `gohawk.path`.
- **No findings where you expect one.** Check the status bar. If the package
  does not compile, or the check was stopped, gohawk says so there.
- **Everything is slow.** Run **gohawk: Show Output** to see how long each
  check takes and how much memory it uses.
