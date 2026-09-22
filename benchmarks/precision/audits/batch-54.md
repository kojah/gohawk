# Batch 54: source review and unresolved evidence boundaries

All 250 pinned repositories were attempted with the unchanged `c9609c6`
baseline across 312 selected modules: 203 completed, 47 were incomplete, and
125 complete scans had no findings. All 311 original findings have individual
source reviews: 242 true positives, 65 false positives and four inconclusive.
Incomplete scans are not clean. These are source judgments, not runtime
reproductions or a measurement of recall.

This is the third of four additional 250-repository batches. After exhausting
the fresh Go 1.26+ pool, selection included older Go declarations; this batch
declares Go 1.24–1.25 and was analyzed with local Go 1.27. Prior audits, cohorts,
outreach and other in-flight selections were excluded. Companion selection
and scan metadata preserve pins, discovery provenance, binary/runner hashes,
module results and errors. Candidate tests, generators and applications were
never executed.

## Verified correction

One of the 65 false positives is verified corrected; 64 remain unresolved.
At `Adembc/lazyssh/internal/core/services/server_service.go:229`, successful
`Cmd.Start` establishes a nonnil `Cmd.Process`. The defensive nil branch is
infeasible, and every feasible continuation launches the exact `Cmd.Wait`
worker. The bounded process-state correction in `88a6b90` recognizes that
successful-start fact without treating arbitrary process-field tests as joins.

The pinned repository revision is
`07cb2abb1abf65d4036a7574c2b33a1808d5614f`. A corrected, all-checks replay of
`./internal/core/services`, including test packages, completed with exit zero
and `{}`. Its binary SHA-256 is
`6affaf921e5ed96304316bdb21b6852d6004c4dab9ede1a4ab6de5453ec072bf`;
the detailed receipt is `.build/audit54-lazyssh-processfix-replay.json`.
This is a scoped successful replay, not a whole-repository rerun. Original
reports and baseline verdicts remain unchanged.

## Check-level reassessment

The following families account for all 64 unresolved false positives. Counts
describe findings, not distinct defects in the analyzer. The per-finding ledger
is authoritative for full pins, positions and evidence.

- **Retained writers and returned owners (nine):** checkrr has two logger
  outputs; openserp, devzat, subtrace and goploy each install a persistent logger
  output. Rest-server returns a logging handler retaining its writer
  (`mux.go:31`), dotweb returns a response context whose release closes its gzip
  wrapper (`server.go:579`), and file.d retains a file in a receiver-owned Job
  (`plugin/input/file/provider.go:382`). Reuse exact retained-owner provenance;
  an opaque registration should remain unknown, not become a name-based
  logging or handler exemption.
- **Parent and callback cleanup (20):** six beans session acquisitions are
  owned by the terminal manager, two hyperpb SSH sessions are covered by the
  exact parent connection's deferred Close, and ipadecrypt registers device
  cleanup in a locally drained callback stack (`cmd/ipadecrypt/decrypt.go:228`).
  Eleven sonic test acquisitions explicitly close the returned UDP peer; its
  guarded Close releases the exact socket. Constructor-internal socket leaks
  on error are separate callee defects, not unfulfilled caller obligations.
  These require exact registration, parent identity and cleanup coverage.
  Shared facts are the appropriate reuse point, but no correction is claimed
  here from later callback work or from merely finding a Close method.
- **HTTP request ownership (one):** agola's test import
  (`tests/api_test.go:3137`) hands the exact file to the standard HTTP request
  body. Earlier successful calls establish the same immutable URL is valid,
  so request construction reaches the transport's body cleanup. The CLI import
  remains a true positive because its configurable URL can fail before that
  handoff. Preserve this distinction; passing a reader to a helper alone is
  not proof of transfer.
- **Correlated cleanup and error paths (six):** three gitmal files are covered
  by deferred cleanup after an exact `err != nil` helper predicate, two
  dns-over-https selectors accept only the two upstream variants whose response
  helpers close bodies, and murphysec's fallback reader closes the original
  file before replacing the cell observed by a deferred closure
  (`module/python/utils.go:19`). Bounded error predicates can reuse shared
  summaries; mutable-cell and variant correlations require stronger evidence.
  Leave uncertain consumption unknown rather than add another local path model.
- **No acquired resource (six):** four easeprobe PID-file tests return before
  acquisition because of an empty filename or explicitly patched failures
  (`daemon/daemon_test.go:98` and subsequent cases). Pmtiles' exact HEAD and
  codapi's exact empty local response use standard zero-timeout clients and
  `http.NoBody`. Do not exempt all HEAD requests, tests or zero-length responses:
  custom transports and timeout wrappers can introduce real cleanup obligations.
- **Worker and producer lifecycle (13):** seven agola service sends are either
  unreachable under the exact Background context or lead directly to the sole
  caller's fatal process exit. Three webwormhole server-race sends likewise end
  at `log.Fatal` (`cmd/ww/server.go:461`). Galene queues a typed shutdown message
  to the exact bounded writer (`rtpconn/webclient.go:879`); murphysec joins all
  stderr work using a second, actually awaited WaitGroup
  (`module/nuget/nuget_cmd_build.go:611`); pion's test sends and receives exactly
  two datagrams before socket shutdown. Exact completion, terminal control flow
  and obligation identity are useful bounded contracts. They do not establish
  that arbitrary cancellation, socket closure or a possible join completes work.
  Cardinality-dependent cases remain unresolved rather than adding loop counts.
- **Lock-state and concurrency context (five):** uTLS has two alerts caused by
  temporarily dropping a read lock, taking the exclusive lock, then restoring
  modes in deferred LIFO order (`common.go:1104,1119`). Its reported QUIC cycle
  conflates a single private handshake with a second concurrent handshake that
  the wrapper does not permit (`u_conn.go:376`). File.d's reverse-order edge is
  initialization of a fresh Job before publication (`provider.go:581`), and
  BounceBack's private worker mutexes are one-use completion gates
  (`internal/proxy/base/proxy.go:155`). Preserve lock modes, deferred execution
  order and publication identity where proven; type-level order edges alone
  cannot establish concurrent participation. These context gaps remain explicit.
- **Collective or single-acquisition loop lifetimes (three):** pmtiles keeps
  input handles alive for merge work after the opening loop (`pmtiles/merge.go:212`),
  paperboy's benchmark opens a one-element table, and nerdlog's rotation defer
  is guarded by a counter changed on its first execution. Deferred cleanup is
  not inherently delayed incorrectly merely because its syntax occurs in a
  loop. Collective ownership and one-shot guards remain unmodeled here.
- **Intentional recovered panic (one):** plow's initializer deliberately sends
  on a closed channel under a dominating recover to obtain a runtime error
  sentinel (`requester.go:53`). The panic does not escape. Any acceptance must
  prove the local recovery boundary, not suppress all channel misuse in code
  containing recover.

No check was disabled, retired or demoted. Unresolved source-proven safe cases
are not silently relabeled as passing corrections. Retain the existing bounded
checks and controls; prefer explicit unknown when their required ownership or
concurrency evidence is unavailable, rather than growing framework exceptions.

## Inconclusive cases and controls

Four rows retain uncertainty rather than assuming a missing syntactic cleanup
proves a persistent leak:

- Nightshift's initial log file (`logs.go:260`) and rollover file (`logs.go:286`)
  are closed on replacement. The follow loop returns only when its private
  watcher channels close; Linux fsnotify 1.9.0 closes them on watcher shutdown,
  while this caller only defers shutdown after return. A feasible Linux return
  leak was not established, and other backend/runtime lifecycle behavior remains
  uncertain. These are two separate reviewed acquisition sites.
- Murphysec's stderr worker (`nuget_cmd_build.go:711`) is joined on successful
  Start; failed Start closes the exec pipes but returns before the explicit
  join. That bounds the I/O source without proving completion before return.
- Zeitwork's container-log worker (`internal/zeitwork/build.go:823`) can miss
  its WaitGroup join on ContainerWait error. Deferred forced removal attempts
  to end the exact log source, but remote removal can fail and only the worker
  closes its reader. Neither full completion nor persistent abandonment was
  established.

True-positive controls include error returns before file/body Close or Cmd.Wait,
failed aggregate construction, real read-lock omissions, shared slice writes,
and unbuffered senders with no remaining receiver. The uTLS disabled-ticket
return really misses its read unlock, despite adjacent false positives. Zoro's
exported server methods can return while peer error senders remain blocked;
unlike the fatal server-race cases, their lifecycles continue.

## Validation and limitations

The ledger assembler validates all 311 finding keys, check IDs, full pins,
checkout HEADs, source files, lines and byte columns. The frozen original binary
SHA-256 is `5b98c2f24b8b5b3b229c2a18bf0728a23fdec1eda891e74bb66c18bc3b3e009e`.
Canonical `make verify`, changed-package race tests and round 56 controls pass
for the coordinated corrections. Those gates do not imply the 64 unresolved
false positives have disappeared.

The optional uncached architecture-package race gate is not claimed clean:
the Go 1.27 / x/tools `go/types` loader race also reproduced from unchanged
`0480106` in an isolated source archive, as documented in the batch 52
assessment. No unrelated analyzer workaround or dependency upgrade is included.
