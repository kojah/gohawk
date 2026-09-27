# Batch 62: 250 fresh repositories, stars 200–249

Batches 62 and 63 were audited together as one 500-repository round with the
`f8c461f` binary (SHA-256
`4b5e83ee699d431e060884d410c434579055621d0b5839991ca49ea77e6e38ea`). That
binary includes the batch-61 fixes (nil selections in the points-to model and
unread done channels in `goroutineownership`) and the retirement of
`use-after-release`, `recursive-acquire`, `channelsafety`, and the four
experimental `producerlifecycle` checks. The [selection receipt](batch-62-selection.json),
[scan report](batch-62-scans.json), [repository ledger](batch-62.tsv), and
[per-finding source review](batch-62-findings.tsv) preserve the exact pins,
coverage status, diagnostic keys, and source-backed verdicts. Batch 63 is
the other half of the round.

Selection used the batch-61 criteria in the 200–249 star band: 356
candidates were reviewed to pin 250, after deduplication against every
earlier cohort. The profile was `-enable-all -gohawk-include-tests -json`
with two scan jobs. The first 135 repositories were scanned with the
shared Go build cache; after it filled the disk, the scan resumed with
isolated four-repository cache windows, which the scan report's cache policy
records. Findings from both parts use the same binary and pins.
Candidate tests, generators, and applications were not executed.

All 250 repositories were attempted across 305 module entries:
199 scans completed and 51 were incomplete. Of the complete
scans, 124 emitted no findings. Incomplete scans keep their exact errors
and any partial findings, and they are **not** counted as clean. Every one of
the 326 emitted findings was reviewed against pinned source: 262 true
positives, 61 false positives, and 3 inconclusive. These are judgments of the reported
policies, not runtime reproductions or a recall measurement.

**Default profile (production files): 102 TP, 20 FP, 2 inconclusive (84%
precision).** Test files, reviewed because the audit profile adds
`-gohawk-include-tests`: 160 TP, 41 FP, 1 inconclusive (80%). A default run
does not analyze test files, so the production figures are what a user of
the default profile sees.

| Analyzer | Production TP | Production FP | Test TP | Test FP |
| --- | ---: | ---: | ---: | ---: |
| `resourcelifetime` | 64 | 12 | 153 | 39 |
| `goroutineownership` | 15 | 0 | 3 | 1 |
| `lockorder` | 6 | 2 | 1 | 0 |
| `processownership` | 2 | 5 | 2 | 1 |
| `concurrentcapture` | 8 | 0 | 1 | 0 |
| `deferinloop` | 2 | 0 | 0 | 0 |
| `producerlifecycle` | 3 | 0 | 0 | 0 |
| `cancellationownership` | 2 | 1 | 0 | 0 |

Inconclusive findings are counted in the totals above but not in the table.

## False-positive families (batches 62 and 63)

- **Test requests that cannot succeed (about 35).** A test forces a call to
  fail, through an always-failing dialer, a missing credential, an injected
  error, or a request to a stopped server, and ends the test with `Fatal`
  otherwise. Seen in golang/oscar, larksuite/meegle-cli, basecamp/basecamp-cli,
  prometheus/common, grafana/ai-sdk, decred/dcrwallet, saucelabs/forwarder, and
  jeroenrinzema/psql-wire. This remains the largest `missing-release` family.
- **In-memory or bodyless responses (about 27).** Fiber's `app.Test` reads the
  response from an in-memory buffer (suquant/wgrest, 18), handlers reply HTTP
  204 or an empty body (prometheus/common, Debian/dcs, grafana/ai-sdk), and a
  test round tripper returns `io.NopCloser` bodies (kubecfg).
- **Cleanup passed as a function value.** A deferred closure or a method value
  such as `f.Close` handed to a logging wrapper closes the resource
  (ubuntu/adsys, koki-develop/gat). The gat sites create the file through a
  generic `Must` helper.
- **Nil-guarded closes.** The body is closed under
  `resp != nil && resp.Body != nil` (Authula, tigrisfs, grafana-plugin-sdk-go).
  This family has now appeared in three consecutive batches.
- **A cache-dependent report.** JiaCheng2004/Polaris
  `internal/gateway/audio_cache_test.go:128` closes the body in a deferred
  closure. The diagnostic reproduces through `go vet` with an empty Go cache
  and not with a warm one, so a fact that reaches this package depends on
  cache state. This is a determinism bug to investigate, not a label question.
- **Ownership handoffs and process-lifetime owners.** A statement stored into
  the store it returns (ferro-labs/ai-gateway), a log file handed to a
  process-lifetime logger or CPU profile (twmb/kcl, golang/debug, urunc), a
  body passed over a channel to its closer (Contextualist/acp), and rows or
  transactions released by drain or context cancellation (jonbodner/proteus,
  OdyseeTeam/odysee-api).
- **`lockorder` helpers that release the caller's lock** (centrifugal/centrifuge-go,
  four findings), a lock and unlock under one construction-time flag
  (pion/sctp), a documented lock handoff (fiorix/go-diameter), and
  single-reader cursor fields (apache/skywalking-rover).
- **`processownership` detached or process-lifetime children.** Daemons
  released with `Process.Release` or `Setsid` (TencentCloud, jayu/rev-dep),
  children started in `main` of example programs (coder/acp-go-sdk, 4), and a
  process stored in a returned container that waits on destroy
  (criyle/go-sandbox).
- **`goroutineownership` workers ended by another mechanism**, such as a forced
  `Stop` or a deferred session close that ends a copy loop (lynxdb, openase).

The three inconclusive `contradictory-order` findings in tigrisdata/tigrisfs
cross parent and child inode locks; whether two such instances can be held in
reverse order depends on filesystem call patterns the source does not settle.
These families are recorded as follow-ups; no analyzer was changed in response
to this round.
