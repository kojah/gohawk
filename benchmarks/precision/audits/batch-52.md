# Batch 52: source review and bounded corrections

All 250 selected repositories were attempted at full pins with source `c9609c6`:
200 completed and 50 were incomplete, across 317 module entries. There were
111 quiet complete scans. All 455 original findings are individually reviewed:
341 true positives, 107 false positives and seven inconclusive. Partial scans
are not clean; these source judgments are not runtime reproductions or recall.

This is the first of four additional 250-repository batches. Fresh Go 1.26+
candidates were insufficient for the additional thousand, so selection expanded
to older compatible versions, newest first. This batch's selected minimum Go
directives range from 1.25 to 1.26.4. Companion metadata records selection,
exclusions, exact revisions, original binary/runner hashes and module failures.
Only static analysis ran on candidates, never their tests, generators or apps.

## Verified corrections

Two of the 107 original false positives are corrected; 105 remain unresolved.
Original verdicts stay unchanged. The correction ledger and round 56 distinguish
review from verified behavior. Separate implementation commits are:

- `88a6b90`: after successful Cmd.Start, an immediate exact Process nil guard
  cannot take the nil return. The bounded flow exemption fixes Stacktower's
  intentional Release path. It requires the direct success edge and unchanged
  command/process projection, not arbitrary distant guards or a name exemption.
  Fixtures retain modified Process fields, other commands and conditional waits.
- `f8365c0`: a direct write to the variable declared by this exact range header
  does not establish storage shared across iterations under Go 1.22+ semantics.
  This fixes envd's worker-local entry mutation. Effective per-file language
  versions are respected. Map-index writes, assignment into outer variables,
  nested-loop sharing and ordinary for-header mutation remain checked.

Round 56 passes all eight labels across four fully scannable repositories:
both corrected false positives are absent, and six controls remain present
(five lazyjournal process omissions and one arkade shared-map write).
The replay is stamped `05eabd6`, with corrected binary SHA256
`0579829a850c737a8bfec1b0eeea0edc36bb5d53a9666b3ddb1d124df9995c13`.
The original audit binary remains the separately preserved `c9609c6` executable.

## Check-level reassessment

The remaining families are retained with explicit evidence gaps, not silently
accepted through project, method-name or framework exemptions.

- **Resource transfer and parent ownership:** global logging wrappers (Geesefs,
  Gokit, BitSrunLoginGo), returned loggers/HTTP wrappers, GitLab's close-or-return
  helper, Mutagen's pre-Start process owner, and SQL transaction-owned rows
  establish lifetimes beyond a local Close. Reuse retained-owner and parent
  lifecycle facts before adding any new summary family. Conditional release
  and nested wrapper provenance still need a bounded shared contract. A returned
  io.Reader or an unrelated closeable object is not sufficient evidence.
- **Acquisition is not always a resource:** Exportarr's synthetic empty bodies,
  Testcontainers' exact empty echo responses, explicit HEAD requests and rejected
  authentication/TLS tests expose acquisition precision gaps. Postgres Exporter's
  constructor closes its temporary validation database and returns metadata with
  no live DB; the existence of a Close method alone is not an obligation proof.
  Standard no-body responses with zero client timeout were distinguished from
  timeout wrappers and body-bearing responses. Retain the check but leave these
  exact acquisition/protocol proofs unresolved; no blanket test/HEAD exemption.
- **Correlated cleanup and feasible paths:** Geesefs' deferred mutable cell,
  Fortio/Mutagen's unchanged profile flags, Minio's paired once.Do cleanup,
  ContainerSSH's exhaustive private state and OpenSurge's disjoint operation
  names all invalidate particular syntactic paths. These need exact relational
  evidence. A broad captured-value exemption would hide vk-turn-proxy's genuine
  overwritten-response cleanup loss, retained as a source-reviewed control.
- **Worker ownership and transitive joins:** Tus and S3 helpers drain channels
  through returned aggregates or downstream APIs; Govmomi's aggregators wait
  after draining accepted inputs, and its adapters return stream ownership;
  golang-set returns an iterator that can be drained or stopped. Conduit joins
  the same workers independently, and zgrab2 closes exact pipe endpoints on
  request failure. Reuse causal completion and exact escape evidence; do not
  infer a join from any nearby cancel, Close or completion-channel name.
- **Lock protection and asynchronous handoff:** gocryptfs' fields are protected
  by a different dominating exclusive content lock, despite a descriptor RLock.
  Fortio deliberately returns a held cache lock with a flag its callers honor.
  Whoami uses an asynchronous mutex release as a semaphore. These are distinct
  evidence models, not reasons to suppress all nested locks or callback unlocks.
  Its separate unprotected worker writes remain true positives.
- **Process/termination boundaries:** direct Process.Wait in test cleanup and
  exact fatal-only caller error paths are missed. Dedicated crash children and
  command-wide signal registrations intentionally live until process termination.
  These reviews are scoped to proven callers and cleanup obligations, not a
  general exemption for main, CLI tools, tests or unjoined work. Normal returning
  file omissions and mandatory process reaping remain controls.
- **Producer protocols:** Govmomi's exact one-object filter emits the initial
  state and one commanded change consumed by two receives. General syntactic
  repetition cannot prove excess sends; a broader cardinality model remains
  unresolved rather than adding event-name or loop-count exceptions.

The seven inconclusive rows retain their specific uncertainty: best-effort cache
work, asynchronous protocol logout, filesystem refresh timing, a test's expected
timeout, bounded server shutdown, and emergency buffer-wipe lock retention.
No check was retired, disabled or demoted. No unresolved false positive was
turned into a passing regression label.

## Validation and limitations

Both changes pass focused fixtures and package race tests. `make verify` passes
after each final implementation, including format/generated checks, module
verification, vet, lint, deadcode, self-dogfood, package tests and the target's
shared-pass race checks. Round 56 passes, and the ledger assembler validates
all original finding keys, check IDs, full pins, checkout HEADs and byte positions.

An additional uncached architecture-package race run exposed a concurrent
`go/types` checker race in the Go 1.27 / x/tools loader. It also reproduces from
unchanged `0480106` in an isolated source archive, before these two changes.
This optional gate is therefore not claimed clean; no analyzer workaround or
dependency upgrade was slipped into this audit. The canonical verify gate and
changed-package race tests pass. Initial replay invocation used a nonexistent
binary path and checked zero labels; only the subsequent successful eight-label
run is counted as verification.
