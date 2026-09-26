# Batch 61: 250 fresh repositories after the lifecycle recall fixes

This batch audits 250 fresh pinned repositories with the `3e8b8ba` binary.
That binary includes the recall-audit fixes: `os.Pipe` ends,
result-guarded deferred releases, resources held in local collections,
and the two new experimental `producerlifecycle` checks for goroutines
left blocked by a return. The [selection receipt](batch-61-selection.json),
[scan report](batch-61-scans.json), [repository ledger](batch-61.tsv),
and [per-finding source review](batch-61-findings.tsv) preserve the exact
pins, coverage status, diagnostic keys, and source-backed verdicts.
Selection used the documented 200–5,000-star expansion. It took 250 of
282 reviewed candidates, after deduplication against earlier audit cohorts.

The binary's SHA-256 is
`d706302ce62e790e23c78778df040c1bf2851829804061de36926e745bfe4ed8`,
and the scan ran with `-enable-all -gohawk-include-tests -json`. The
runner's SHA-256 is
`48def2c5ca03ef09d74cc30c4052b0fb7d69f3555908cba678aaa58ad26582cb`.
Unlike [batch 60](batch-60.md), which audited the v0.4.0 default, this
profile includes test files. Candidate tests, generators, and
applications were not executed.

All 250 repositories were attempted across 332 module entries. 189 scans
completed and 61 were incomplete. Of the complete scans, 101 emitted no
findings. Incomplete scans keep their exact errors and any partial
findings, and they are **not** counted as clean. Every one of the 398
emitted findings was reviewed against pinned source: 331 true positives
and 67 false positives. These are judgments of the reported policies, not
runtime reproductions or a recall measurement.

| Analyzer | TP | FP |
| --- | ---: | ---: |
| `resourcelifetime` | 219 | 36 |
| `goroutineownership` | 53 | 10 |
| `lockorder` | 16 | 18 |
| `deferinloop` | 14 | 1 |
| `processownership` | 9 | 1 |
| `concurrentcapture` | 8 | 1 |
| `producerlifecycle` | 8 | 0 |
| `cancellationownership` | 4 | 0 |

## New producerlifecycle checks

`producerlifecycle/unreceived-return` reported eight sites, and all eight
are true positives. In each one, a select arm (a timeout, a context done
or a client disconnect) or an early error return leaves an unbuffered
channel unreceived, so the worker blocks forever on its send. Seven of
the eight are also reported by `goroutineownership/unjoined`. The one
unique site is the Envoy exit channel in `esp-v2`.
`producerlifecycle/unsignalled-receiver` reported nothing.

## False-positive families

- **`lockorder/read-lock-write` on locally built values (14 FP, 8 TP).**
  A write under a read lock goes to a slice or struct that the function
  allocated itself, such as a shuffled result slice or a locally built
  task list, not to state the lock guards. Seen in `ipfs/boxo`,
  `kerlenton/mcpsnoop`, `mgtv-tech/redis-GunYu`, and `thushan/olla`. This
  is the largest family by rate. The check should decline writes whose
  base is a fresh local allocation that has not escaped.
- **Paths that a fatal assertion or a provably failing call rules out.**
  A test requires a query to fail, or asserts with a fatal helper when a
  success value is non-nil, so the leaking path cannot complete. Seen in
  `snowflakedb/gosnowflake` (including a query on a closed `sql.DB`) and
  `anatol/pacoloco`.
- **Cleanup that closures or ownership handoffs perform.** A deferred
  closure drains and closes the body (`ember`). A deferred loop closes a
  slice of readers (`mkbrr`). A refcounted logger ref closes on the last
  release (`gkit`). A response writer takes the body (`goscrapy`). Drain
  goroutines close a pipe's read end (`agentsh`).
- **Nil-guarded closes and deferred conditional unlocks.** `PasarGuard`
  and `smokedmeat` close under `resp != nil`. In `opensearch-go`, a
  deferred unlock is guarded by a holding flag, and a lock is handed to a
  callee that unlocks it.
- **Goroutines that end through another mechanism.** A test cleanup
  closes the pipe that a drain loop reads (`agentsh`). A callee closes
  the status channel (`dalec`). A worker's own timer or a forced `Stop`
  ends it (`muninndb`). Process exit ends the workers (`frontier`).
- **Single findings.** Cooperatively scheduled `ctx.Go` coroutines
  (`durabletask-go`). A `deferinloop` over two drained row sets (`kd`).
  A deferred nil-guarded Kill and Wait (`wrr/drop` processownership). A
  child process that exits on the error path (`wrr/drop` ptys).

These families are recorded as follow-ups; no analyzer was changed in
response to this batch. A finding's original verdict is not rewritten by
a later corrected replay.
