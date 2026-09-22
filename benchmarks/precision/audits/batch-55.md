# Batch 55: shared callback summaries and remaining boundaries

This is the fourth additional 250-repository batch, following the initial
250-repository batch 51. Original scans use the unchanged `c9609c6` binary;
the corrected binary is separate. Selection includes older compatible Go
declarations after exhausting fresh Go 1.26+ candidates. Companion manifests
record full pins, exclusions, module limits, failures and static-only execution.
Candidate tests, generators and applications were never executed.

All 250 repositories were attempted across 292 module entries: 194 completed,
56 were incomplete, and 123 complete scans were quiet. All 592 original findings
are individually reviewed: 520 true positives, 69 false positives and three
inconclusive. Ten false positives have verified corrections; 59 remain unresolved.
Incomplete scans are not clean, and these source judgments do not measure recall
or demonstrate runtime incidents. Repeated generated-code findings are not
independent bug classes: gotgbot contributes 175 instances of its generated
return-value/Unmarshal operand-order pattern, plus one resource omission.

## Verified shared-infrastructure correction

Commit `7abe771` preserves bound cleanup callbacks in exported lifecycle
summaries. Umoci's `VerifyClose` passes `closer.Close` to `VerifyError`, which
invokes that exact callback synchronously. The local completion engine could
already follow this behavior, but the package-boundary summary lost it.

The existing summary decision now combines the exact receiver's bound-method
proof with the existing callback-invocation completion proof. It requires a
single exact receiver capture, a generated bound-method wrapper, a synchronous
call, and invocation on every normal return. A conditional call, replacement
callback, different receiver or asynchronous launch does not satisfy it. The
search through the invoking helper is budgeted; arbitrary closure bodies are
not newly searched for each parameter and lifecycle method.

This reuses shared completion machinery, not a new analyzer-specific traversal
or a function-name exemption. It benefits consumers of the exported lifecycle
method masks. No changes are claimed for opaque registries, callbacks stored
for later execution, or deferred mutable-owner cells.

Round 57 replays the whole pinned umoci repository successfully: ten reviewed
false positives are absent and two genuine cleanup omissions remain. Two
permission-negative test false positives remain among the corrected run's four findings;
they are not mislabeled as passing regressions. The twelve labels are stamped
`7abe771`. Corrected binary SHA-256:
`0bb0a98afd53774e2489af825de142012fbe0352b3e7fa37ca7c21f0b41492c0`.
The per-location correction ledger preserves the original verdicts separately.

## Check-level reassessment

Other false positives remain unresolved rather than receiving project or API
name exemptions. The final per-finding ledger is authoritative for exact pins,
locations and any uncertainty.

- **Returned and retained owners:** ccNexus returns the response body behind a
  Reader interface; fastschema retains a file in a returned zap logger;
  selfupdate returns a process owner whose Close waits for the child. Reuse
  exact retained-owner and cleanup summaries. An interface conversion or
  unrecognized wrapper is not itself proof of abandonment. Quickfix's successful
  message-file acquisition is retained in the returned logger, while its earlier
  event-file acquisition still leaks if opening the second file fails.
- **Indirect cleanup:** kube-no-trouble closes option-held files through
  synchronous subtests and their returned printers. Circuit cancels the exact
  request context on its early copy-error path, and dungbeetle cancels the exact
  query context, which ends database/sql rows. These require specific parent
  contracts and identity; a nearby Close or cancel must not excuse any resource.
  OpenKruise's response is closed by a local nil-guarded closure on both status
  paths. This direct captured effect remains unmodeled; it is not covered by
  the narrower bound-method callback correction.
  Three ch.at selftests pass the exact response/error pair to a helper that
  closes every successful response; its separate non-200 error paths remain
  genuine cleanup omissions. This needs correlated acquisition and helper
  effects, not an unconditional cleanup claim about every helper invocation.
- **Acquisition and path feasibility:** umoci's permission-negative tests cannot
  acquire a file on the intended path; enc's raw acquisition-error sentinel
  guard cannot run after successful Open. Pion ICE's exact peer protocol and
  Gorsk's preflight response produce no-body responses; Bookget's HEAD path
  likewise differs from its separately closed range GET. Standard NoBody does
  not justify blanket acceptance of all HTTP responses, especially timeout
  wrappers and custom transports.
- **Cancellation:** NELM's locally derived timeout contexts are covered by the
  exact parent's immediately deferred cancellation. Preserve parent identity
  across context reassignment through shared provenance; do not infer cancellation
  from names or from a context that might be related. Sponge's test waits a
  mandatory second after creating a 100ms timeout; the deadline has already
  expired before return. This reviewed test does not justify a general timing
  model or treating arbitrary deadlines as prompt cleanup.
- **Non-releasing cleanup methods:** two Sponge Gemini wrappers delegate Close
  to genai v0.19.0, whose three cloud.google.com/go/ai v0.8.0 REST client Close
  methods only clear their HTTP client fields. They release no owned resource.
  A separately allocated cache gRPC client is omitted by upstream Close;
  recommending the wrapper Close cannot repair that separate upstream issue.
  A Close-shaped method alone must not establish a cleanup obligation.
- **Worker and process completion:** Pat waits through errgroup; httptap
  delegates exact process Wait to its worker. Goflow's completion follows
  consumption of all input sends. Arrakis closes the exact connection that
  blocks its reader, while a finite delayed producer deliberately outlives a
  latency assertion and exact pipe closure releases its writes. These are
  different completion contracts, not interchangeable
  reasons to consider arbitrary goroutines joined.
- **Termination boundaries:** WireGuard's daemon parent exits immediately
  after its standard descriptors are inherited; the OS closes the originals,
  not Process.Release. Codesearch's command has an exact immediate exit caller,
  unlike its repeated HTTP handler. Driftctl's only caller exits immediately
  after an error return abandons its optional version notification. Keep these
  concrete caller boundaries separate from an exemption for all main functions.
- **Lock identity, serialization and handoff:** Pion DTLS uses distinct peer
  connections; WireGuard's particular inverse paths share an outer exclusive
  configuration lock or drop the exact target's read lock first. Driftctl's
  cache counter is serialized by a keyed mutex. ACME-DNS hands Unlock to the
  launched server's exact startup callback, a permitted cross-goroutine release.
  Yamux joins its receive worker before taking the reverse lock order. A common
  type, callback or channel alone does not establish these facts. Preserve exact
  identity and causal order, or leave the check uncertain. WireGuard's outer-lock
  judgments concern concrete in-repository callers, not every possible external
  caller of its exported helpers. Gopcua's renewal cycle similarly conflates
  the old channel instance with the fresh, not-yet-published opening instance;
  its separate subscription-service lock cycle remains a true-positive control.
- **Actual multiplicity:** Clusternet's four role-worker captures each range
  an exact one-element helper result, with Wait separating the two phases;
  additionalRoles returns an empty slice. There are no concurrent writers to
  the local error slice. Its five separate shared-error worker assignments are
  genuine controls. Do not turn this review into a general loop-count heuristic.
- **Once and startup semantics:** ctrld's global OnceValue initializer is
  evaluated only once, unlike its callback that constructs a fresh OnceFunc on
  each invocation. The latter remains a true-positive repeated-close hazard.
  Its startup test also uses the exact registered Unlock callback, like ACME-DNS;
  that is asynchronous handoff, not synchronous completion proved by the fix.

Retain the existing bounded checks while documenting these missing evidence
families. No check was disabled, retired or demoted, and no unresolved false
positive was silently turned into a passing regression label.

## A source-backed queued-writer hazard

WireGuard's `device/noise-protocol.go:542` is different from its three false
lock-order alerts. A feasible three-participant interleaving involves two
handshake workers for the same outstanding response and a private-key update.
At least two handshake workers are required; both messages have the same Receiver
index and resolve to the same handshake instance:

1. One worker finishes read-locked validation and pauses before acquiring the
   handshake write lock. Another holds that handshake's read lock and pauses
   before acquiring the device identity read lock.
2. The updater acquires the identity write lock. The second worker now waits
   for identity while retaining its handshake read lock.
3. The first worker queues a handshake writer. The updater's new handshake
   reader is blocked behind that writer, closing the dependency cycle.

This uses Go RWMutex's queued-writer behavior; two readers alone would not
establish the cycle. Handshake workers do not hold the outer configuration
mutex that serializes the other reviewed paths. The pinned source shows
[parallel workers](https://github.com/WireGuard/wireguard-go/blob/ecfc5a8d54462e18e13c72173e2623d16d8e25a0/device/device.go#L311),
[separate handshake lock phases](https://github.com/WireGuard/wireguard-go/blob/ecfc5a8d54462e18e13c72173e2623d16d8e25a0/device/noise-protocol.go#L533),
and [updater lock ordering](https://github.com/WireGuard/wireguard-go/blob/ecfc5a8d54462e18e13c72173e2623d16d8e25a0/device/device.go#L232).
This remains a source-backed hazard, not a reproduced production incident;
no candidate application or runtime deadlock experiment was executed.

## Inconclusive findings

Three findings retain their uncertainty:

- Gokeyless's empty Group path omits an RUnlock, but its constructor rejects
  empty groups and all repository callers use that constructor. The public
  zero-value contract and a concrete violating caller were not established.
- Distri's virtual-directory type-assertion failure returns with a mutex held,
  but reaching that branch through valid kernel directory requests was not
  established from its inode construction and advertised types.
- Overseer's HEAD response has no payload, but its nonzero client timeout can
  wrap NoBody in a cancelTimerBody. Missing Close can retain a bounded timer;
  a persistent transport leak was not established and actionability is uncertain.

## Validation

The callback correction passes minimized summary-boundary tests, resource
fixtures, both changed-package race tests and canonical `make verify`.
Rounds 55 and 56 retain all 29 earlier labels; round 57 passes all twelve new
labels. Original scan hashes are unchanged. Ledger validation checks every
finding key, check ID, full pin, checkout HEAD and source position.

The optional architecture-package race gate is still not claimed clean: the
Go 1.27 / x/tools type-loader race was reproduced on unchanged `0480106`, as
recorded in batch 52. No unrelated dependency change or analyzer workaround
was included to conceal that limitation.
