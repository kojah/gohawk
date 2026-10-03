# Consolidated production FP queue refresh

Beads `gohawk-dho.4.9` refreshes every original key in the fixed 55-site
[production queue](production-fp-queue-2026-10-01.tsv) after the shared-engine
consolidations. The [current site ledger](consolidated-production-fp-refresh-2026-10-03.tsv)
retains the original pin, verdict and status beside current diagnostics,
target decisions, cutoff events and the next action. Frozen labels are unchanged.
This is the specified production queue, not the full precision-regression corpus
or a new label audit of every historical batch.

## Inputs and execution

The immutable `.build/goal-lock-modes-reviewed` executable implements production
source at `e3957de96e80ebe81804b6ab8dd563f76e8c5be0`, with SHA-256
`b85da01e465b36ea308902db0d91ba22226e62eec1b8ccfee92f83b845f8122a`.
Later `58cba6f3` changes only the development reconciliation document.
Every candidate checkout matched its recorded revision and had a clean working
tree before its invocation. Twenty-nine package scopes across twenty-nine
repositories cover all 55 original sites. Every scope terminates with exit zero
and empty stderr. Scopes are derived from the nearest containing module;
FDio uses `extras` and the vendored speedtest module is scanned separately.

Commands use `go vet -vettool=<immutable binary> -enable-all -json`, selecting
trace check IDs from that scope. `CGO_ENABLED=0`, `GOWORK=off` and
`GOFLAGS=-mod=readonly` remain fixed. At most two scans run together.
Candidate tests, generators and applications are not executed. Inputs, complete
JSON, JSONL traces and terminal receipts are in
`.build/goal-completion-queue-e395/`. The reconciliation uses complete JSON
payloads, exact original source/check keys, and separately records same-line
column matches. No such column shift occurs in the four retained findings.

## Current observations

| Original production sites | Count |
| --- | ---: |
| Absent in a successful current package scan | 51 |
| Existing goiardi FP targets still reported | 2 |
| Returned-logger FP reappeared in urunc | 1 |
| Original Promu review corrected to a true positive, still reported | 1 |
| Total | 55 |

The 51 silences include Openase's two unresolved transport sites and Ferro's
unresolved process site. They remain model assessments, not corrections.
Twelve target traces contain cutoff events; the ledger records their reason
counts without inferring that every cutoff was decisive. Some silences occur
before a final candidate decision. None receives new FP-correction credit.
The other 48 absent sites retain their historical correction or policy receipts,
with this current package observation kept separate from those earlier proofs.

The Promu leak at `cmd/release.go:191:14` still ends in `unowned-return` and
is reported. The two goiardi sites at `shovey/sql_funcs.go:460:13` and `492:13`
retain the same rejected result. Existing actual SSA probes still distinguish
caller-fixed snapshots from mutable global reloads; this scan provides no new
caller-precondition or mutation-stability guarantee.

## Reappeared returned logger

The earlier [round 69 correction](../round-69/README.md) verified that urunc's
`internal/metrics/metrics.go:61:16` became silent. The current successful scan
reports it again, with `unowned-return` and no target cutoff. This contradicts
completion based on retaining the old absence receipt. Beads `.4.9.1` owns the
investigation and correction; the broader goal remains active.

Actual SSA still stores the final returned logger value into a local cell and
puts that cell in the returned owner. Current imported facts preserve writer
field relations through `Level`, `With` and `Logger`, but `Timestamp` publishes
unknown may edges for the nested logger's reference fields. The old scalar-copy
correction is present; the remaining loss must be traced through the intervening
call and its uncertainty handling rather than patched with a logging name.
Artifacts: `.build/goal-urunc-e395.ssa`, `.build/goal-urunc-e395.facts` and
`.build/goal-urunc-e395-target-trace.json`. These locate the evidence loss;
they do not yet identify its introducing commit or prove the correction.

## Separate Rune controls

The extra Rune invocation initially named a nonexistent workspace subpackage.
Its load error remains recorded in the queue run; it contributes no validation.
The corrected invocation scans `./internal/ide/idepkg` and `./internal/workspace`
at `3e2165f8983280542c985947378dfa740a397d03` with `CGO_ENABLED=1`, the same
immutable binary and read-only flags. It terminates with exit zero and empty
stderr. Its artifacts are `.build/goal-completion-rune-e395/`, including the
replacement receipt and complete trace. These controls are outside the 55-site
queue tally. Both original targets remain absent. The workspace target records
`pre-start-ownership-unknown`, and the idepkg target has no matching final
decision at its original line. These observations retain the earlier bounded
correction receipts without crediting a new joining guarantee. Target records
are in `target-results.json`; loading successfully alone proves no ownership.
