# Rune callback waiter correction

Correction `0acc8ef` shares the existing opaque callback handoff rule between
ordinary calls and imported or unresolved runners launched with `go`.
At [rune's pinned waiter](https://github.com/unstablebuild/rune/blob/3e2165f8983280542c985947378dfa740a397d03/internal/workspace/file_scheme.go#L451-L467),
the command is captured by the callback passed to the imported panic-reporting
wrapper. Its invocation guarantee is unavailable; this is unknown ownership,
never guaranteed invocation or cleanup. Visible runners that drop callbacks,
callbacks capturing another command, and returns bypassing the handoff retain
diagnostics. Dynamic and imported-wrapper fixtures reproduce before the change
and pass afterward; trace assertions require the unknown decision.

## Scoped evidence

The parent binary implements `bc39b44`, SHA-256
`6c54d1d7a22043db52398de1fcb68a692eca7cca6fd0bf911cb61b1fdd453299`.
The corrected binary was built before commit and implements `0acc8ef` production
source, SHA-256
`c60bc0ecf5dfc5cfd35ebd3c546aea91d73916aee4225f63eb518ea6a79e8971`.
The [ledger](rune-callback-waiter-followup-2026-10-01.tsv) records pins, scopes,
profiles and receipts. Scans use `go vet -vettool=<immutable binary> -enable-all
-json`, `GOFLAGS=-mod=readonly`, and `GOWORK=off`. Candidate tests, generators
and applications were not executed.

| Scan | Result |
| --- | --- |
| Rune workspace, parent, CGO disabled | Exit 0; reviewed missing-wait present. |
| Rune workspace, corrected with tracing, CGO disabled | Exit 0; only finding difference is removal of `file_scheme.go:451:8`. |
| OpenFaaS complete repository, corrected, CGO disabled | Exit 0; reviewed `serializing_fork_runner.go:105:12` process leak retained. |
| Rune IDE, parent, CGO disabled | Exit 1; missing tree-sitter declarations, no absence credited. |
| Rune IDE, parent, CGO enabled | Exit 0; separate contradictory-order report at `manager.go:422:9` remains. |

The workspace trace in `.build/goal-rune-workspace-corrected.trace.jsonl`
records `ambiguous-wait-ownership`, phase `decision`, outcome `unknown`, at
`file_scheme.go:451:20` (the same acquisition expression as the diagnostic).
The corrected JSON and trace establish the classifier boundary rather than
merely observing an absent report. Successful scan stderr receipts are empty.
Final `make verify` passes; no full precision-regression corpus ran.

Beads `gohawk-tz7` owns this correction. Rune's publication-order lock issue
`gohawk-cnx` remains unresolved: its fresh mutex is published before its first
Lock under the map guard, requiring participant/guard evidence beyond ordinary
unescaped-allocation identity. Its diagnostic also retains an SSA identity in
the displayed name. Enabling checks in rune is separate work.

Rune is outside the bounded production queue from batches 62 and 63. This
follow-up changes neither its frozen labels nor its remaining 20-site count.
The consolidation objective remains active.
