# Batch 60: first audit of the v0.4.0 release

This is a fresh 250-repository tranche scanned with the released v0.4.0
binary. The [selection receipt](batch-60-selection.json), [scan
report](batch-60-scans.json), [repository ledger](batch-60.tsv), and
[per-finding source review](batch-60-findings.tsv) preserve exact pins,
coverage status, diagnostic keys, and source-backed verdicts.

Selection used the batch-59 criteria: Go, not a fork, not archived, 200 to
5,000 stars, under 30,000 KB, and a pinned root `go.mod` whose `go`
directive is 1.20 through 1.27.0. GitHub search results, sorted by recent
update within star bands, were deduplicated against every earlier selection
ledger and regression cohort; 344 candidates were reviewed to pin 250. Only
GitHub metadata and the root `go.mod` were read during selection.

The analyzer was the v0.4.0 release binary built from `89e0bcd` (SHA-256
`f446a901d6562fb12a29c0ea40214a5d40b9936c05549f7f3c1460dd87f88f4e`) with
`-enable-all -json`. Unlike batches 56 to 59, the profile omits
`-gohawk-include-tests`: gohawk no longer analyzes test files by default,
and this batch audits that default. The scan used two jobs, isolated
four-repository Go build-cache windows, and a 12 GiB free-disk guard.
Candidate tests, generators, and applications were not executed.

All 250 pinned repositories were attempted across 294 module entries: 189
complete and 61 incomplete scans, with no failed repository report.
Incomplete scans retain their exact errors and emitted partial findings;
they are **not** counted as clean. Every one of the 144 emitted findings was
reviewed against pinned source: 122 true positives and 22 false positives.
These are judgments of reported policies, not runtime reproductions or a
recall measurement. The new `producerlifecycle/stopped-loop-send` and
`unclosed-range` checks reported nothing.

| Analyzer | TP | FP |
| --- | ---: | ---: |
| `resourcelifetime` | 83 | 10 |
| `goroutineownership` | 9 | 6 |
| `deferinloop` | 8 | 0 |
| `concurrentcapture` | 7 | 2 |
| `lockorder` | 7 | 1 |
| `processownership` | 6 | 3 |
| `producerlifecycle` | 2 | 0 |

One repository, `neilalexander/yggmail`, contributes 24 resource findings
from one family: each table constructor prepares statements into a
half-built struct and returns `nil, err` when a later `Prepare` fails,
leaking the earlier statements. It must not be mistaken for 24 independent
bug families.

## False-positive families

| Family | Findings | Analyzer |
| --- | ---: | --- |
| A logger keeps the file as its writer: returned `slog`/`charmbracelet/log` loggers, `slog.SetDefault`, and a `log.New` logger stored in `http.Server.ErrorLog` | 5 | `resourcelifetime` |
| An early return guarded by a condition the path already settled: a status code retested after it was proven 200, and a helper that returns a nil result whenever its error is non-nil | 3 | `resourcelifetime` |
| A body stored in a returned wrapper whose `Close` closes it | 1 | `resourcelifetime` |
| `f.Close` held in a local method value that a deferred closure calls | 1 | `resourcelifetime` |
| A fixed-count `select` loop that receives once from each of five workers | 5 | `goroutineownership` |
| The sole caller passes the result straight to `os.Exit` | 1 | `goroutineownership` |
| A loop bound fixed to the constant 1, so only one worker writes the captured local | 2 | `concurrentcapture` |
| The command is waited on by a goroutine a helper launches; a detached hot-restart relaunch; a command stored in a package map that a later function waits on | 3 | `processownership` |
| A constant argument disables the callee's lock, so the cited reverse acquisition cannot happen; a real inversion exists on another path | 1 | `lockorder` |

The select-loop and constant-1 families depend on a loop count, which the
analyzer policy does not argue; the process-exit family rests on the open
process-exit policy decision. Retired and removed checks are absent from
this batch, so its totals are not directly comparable with batches 56 to 59.
Original verdicts are not rewritten by later replays.

## Follow-up fixes

A corrected binary built from `da6624a` was replayed on the 58 repositories
with findings, under the same profile. Exactly the nine false positives below
disappeared; every true positive remained, and no new finding appeared. The
original verdicts above are unchanged.

| Fix | Findings |
| --- | --- |
| `003656a`: a constructor chain over a resource is handed over where a callee is proven to store it, such as `slog.SetDefault`, or when stored on an object the function did not allocate; the heap graph applies `sync/atomic` stores as stores | VibeGuard ×2, grok-build-switch |
| `ee11ebd`: two checks of the same field of a call's result relate, until the call runs again | woodpecker ×2 |
| `28b56ae`: an acquisition no feasible path reaches is skipped; `return nil, err` satisfies the result-nil-when-error relation | get-sauce |
| `fa1736e`: a started command stored into a package variable or registry map is handed over | router |
| `43a641b`: an abandoned completion search stays undecided | kilroy |
| `da6624a`: a callee's locks are read under the constant Boolean arguments a call passes | libovsdb |

The remaining false positives stay open. The returned `slog` and charm
loggers (opsy, contrabass) are structurally a returned writer wrapper, which
is reported deliberately, and need a policy decision; depot rests on the open
process-exit decision; 115driver carries its constant loop bound through a
struct field, which is value provenance rather than a loop count; maxigo and caam are single shapes that do not
yet justify a new proof. The l3afd verdict is disputed: its failure paths kill
the relaunched child without waiting on it inside a server that keeps
running, which leaves a zombie, so the finding is likely a true positive.

A later change clears the five go-quests findings with a counted select
drain: a loop that runs a blocking, receive-only select exactly N times over
N channels that each have at most one send must have drained every sender
before it exits. The pinned `select_timeout.go` no longer reports.
