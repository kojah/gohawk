# Gohawk precision follow-up handoff — 2026-09-22

## Start here

The user asked to stop this session, commit validated work, and hand off to
Claude because the follow-up was taking too long. Do not restart a new audit
batch or assume every remaining finding warrants another analysis model.

Repository: `/home/james/scratch/gohawk`, branch `main`, upstream `origin/main`
(`https://github.com/kojah/gohawk.git`). The validated implementation checkpoint
before this handoff is `1fc6f76`; the handoff itself is a later documentation
commit. Implementation changes have been committed and pushed incrementally.

Read `AGENTS.md` and the applicable `.agents/skills/` instructions before edits.
Use the codebase graph for structural discovery, check coverage, and inspect
actual SSA/traces rather than mentally compiling candidate source. Prioritize
existing shared infrastructure and conservative classifier uncertainty over
new proof families. Do not silently weaken controls, change original verdicts,
or treat a failed scan as a correction.

## Authoritative scope and counts

The immutable input is
`benchmarks/precision/audits/followup-207-input.tsv`: **207** reviewed false
positives left after earlier work on the 337-site follow-up. These are findings,
not 207 repositories or separate implementation projects.

| Family | Original | Verified fixes | Re-reviewed as a real hazard | Open |
| --- | ---: | ---: | ---: | ---: |
| Resource lifetime | 140 | 20 | 0 | 120 |
| Lock checks | 23 | 13 | 0 | 10 |
| Goroutine ownership | 17 | 8 | 0 | 9 |
| Process/cancellation | 9 | 4 | 0 | 5 |
| Miscellaneous | 18 | 1 | 1 | 16 |
| Total | 207 | 46 | 1 | 160 |

One open resource site is absent under both current and baseline profiles; its
inconsistent earlier reproduction is unresolved, not a fix. The Sloth review
correction is **not** a suppression or diagnostic removal. Preserve the original
frozen label and the later explanation.

Read these family records rather than inferring status from old aggregate prose:

- `benchmarks/precision/audits/followup207-resources.{md,tsv}`
- `benchmarks/precision/audits/followup207-locks.{md,tsv}`
- `benchmarks/precision/audits/followup207-goroutines.{md,tsv}`
- `benchmarks/precision/audits/followup207-process-cancel.{md,tsv}`
- `benchmarks/precision/audits/followup207-misc.md`

The original `followup-78-*`, `followup-337-*`, and batch-51 through batch-55
ledgers retain historical verdicts and context. Do not add their correction
counts to this 207-site denominator.

## What is already shipped

Representative checkpoints (see commit diffs and family reports for details):

- `511ed2b`: edge-local select ownership; exact non-nil value identity; goroutine
  transport/relay/cleanup uncertainty; process successful-Start merge handling.
  These changes were committed together because splitting their dependencies
  lost known-bug controls.
- `0596236`, `b7f5f9b`, `df6166f`: exact testing/process termination contracts,
  dominating deferred termination at RunDefers, and bounded literal helper
  result feasibility. A defer registration or `go` call does not terminate the
  current function.
- Resource checkpoints through `5792a2b` and audit `2d0c9ac`: returned/retained
  owners, manager retention uncertainty, private logger retention, exact error
  predicates through immutable captures, narrowly bounded HTTP cases, and
  standard buffer-constructor parity for compression writers.
- Lock checkpoints through `1fc6f76`: bounded guard state, conditional caller
  release, shared embedded-field paths, fresh owner-slot uncertainty, and
  narrower cross-owner class widening. Exhausted lock flow discards both its
  diagnostics and order edges transactionally; never publish partial proofs.
- `8855530`: uncertain JSON header replacement of a fresh otherwise-unused
  plain map is not reported. Nil maps, entry snapshots, custom decoders, and
  explicit replacements retain diagnostic fixtures. Null-driven header resets
  are a documented coverage loss, not claimed safe.
- Guarded captured cleanup (after this handoff): a called literal that
  nil-guards an exact captured `http.Response.Body` before closing it is
  unknown consumption, never proven release. Kruise is corrected; the full
  83-scope replay retained 66/66 controls with zero new diagnostics.
- `fb3b8cf`, `07b8cab`: Sloth's output handles are not always retained for later
  generation. Empty/comment-only YAML produces zero inner iterations, leaving
  unused outputs open across the outer loop. Existing behavior is preserved
  with a minimized fixture; the original FP judgment is corrected explicitly.

## Validation and its limits

- Final restored-source `make verify` passed at handoff:
  `.build/followup207-handoff-verify.log`. The archived goroutine patch passes
  `git apply --check` against this checkpoint. No audit workers or scan
  processes remain running; source is restored to the validated commits.
- `make verify` passed for the combined lifecycle checkpoint and again for the
  evalorder checkpoint. Logs: `.build/followup207-verify-final-checkpoint-v2.log`
  and `.build/followup207-evalorder-verify.log`.
- Resource replay (captured-body-v3): all **83** scopes; **66/66**
  baseline-detected bug controls retained, six historical misses unchanged,
  zero new resource diagnostics, twenty removed.
- Lock replay v19: all **20** scopes; **4/4** controls and both earlier fixes
  retained; zero new lock diagnostics.
- Shipped goroutine select-v10: all **66** scopes; **56/56** old bug controls
  retained, 23 earlier fixes retained. Three newly exposed genuine bugs remain
  reported; ten newly exposed FPs were fixed. The ledger has 109 reviewed sites.
- Process/cancellation: **13** scopes; **7/7** bug controls and two earlier
  corrections retained.
- Evalorder historical replay: seven TP labels retained, six FP labels remain
  absent. The overall command fails on the pre-existing round-10 label-count
  mismatch. Agent-beacon's labelled package was recovered despite a separate
  missing-assets package. Do not call the cumulative historical suite green.
- Round 58 replay completed: **70 checked labels pass** (60 FP absences, ten
  TP controls); **25 labels in six incompletely loadable repositories excluded**.
  Log: `.build/followup207-round58.log`. This is not a 95-label clean scan.
- Older historical gate failures and measured coverage losses are documented
  in `benchmarks/precision/audits/followup-337-historical-validation.md`.

## Uncommitted experiments: not part of the validated implementation

The stopping procedure saved in-flight patches, removed only their authors'
uncommitted source changes, and stopped their scan processes. One patch remains
committed as an inert archive, **not applied source**, under
`benchmarks/precision/audits/followup207-candidates/`:

- `goroutine-field-guard.UNVALIDATED.patch`

The captured HTTP cleanup candidate has since been validated and shipped, so
its archive was removed; the committed source is authoritative.

Use `git apply --check PATH` before considering the archive; the current
committed source is the baseline, not the rejected candidate. The goroutine
archive contains the later unvalidated repair, not the exact failed v2 binary.
Detailed goroutine notes also remain locally at
`.build/followup207-goroutines-remaining-handoff.md`. Raw receipts, binaries,
checkouts, and SSA dumps are local-only; retain this checkout to reuse them.
Additional resource notes are `.build/CLAUDE-resource-followup207.md`.

### Goroutine repeated field guards — rejected candidate

The Ghostferry candidate recognized matching receiver-field nil guards around
launch and later join. Its first full replay retained only **54/56** old TPs:

- Rootlesskit `pkg/parent/parent.go:302`: a global guarded-join exemption hid an
  early return before the later join. Any future repair must attach uncertainty
  to the later CFG action and let ordinary flow preserve earlier unowned exits.
- Skupper `pkg/vanflow/eventsource/client_test.go:242`: extending local context
  factories to WithTimeout/WithDeadline hid a completion-send obligation.
  Expiry/cancellation alone does not settle a later channel send.

A path-local revision was being drafted but was **not fully replayed** at the
stop. Do not apply the patch wholesale or count Ghostferry as fixed.
Shared `lifecyclefacts/evidence.go` was not changed by this experiment.

### Captured HTTP cleanup — validated and shipped

Kruise's helper captures an exact response and nil-guards Body.Close. Distinct
loads prevent the completion proof, so the shipped change classifies the exact
guarded captured cleanup as unknown, not guaranteed release, with mutation and
extra-condition controls. The full 83-scope resource replay, race run, lint,
architecture tests, and `make verify` all passed after the handoff. The
existing captured-cleanup family moved cohesively into `captured_cleanup.go`.

## Remaining work, grouped rather than one model per finding

Resources (120): 45 memory/empty HTTP bodies; 19 correlated errors/guards;
18 failure-fixture assumptions; 11 process-bounded lifetimes; eight returned or
retained owners; seven aggregate cleanup; five logging outputs; seven smaller
cases. The 18 failure-fixture cases are mostly **not** assertion helper gaps:
they include injected transports, redirect rejection, monkey patches,
filesystem/permission assumptions, TLS, connection faults, and timing.
Potential reusable work is exact transport/handler callback evidence, not a
blanket exemption for tests expecting errors.

Locks (10): gauge initialization before publication; uTLS QUIC protocol; yamux
join ordering; two WireGuard device cycles; ContainerSSH private state domain;
two template-error paths; OpenSurge correlated predicates; gophercloud callback
cardinality. Yamux's receive is visible, but its worker is reached through a
private dynamically indexed handler table. Recognizing the receive alone is
not enough. No private-state/string solver or handler-table model was added.

Goroutines (9): Clawk transport shutdown (two), gonc nested callback/socket
lifetime, Kadeessh watchdog completion, k8s-csi-s3 worker-published error,
my-geektime receiver context, two CLI process-lifetime cases, Ghostferry guard.
my-geektime's SSA shows exact standard Context.Done on a context field of the
caller-owned receiver; existing receive traversal may be reusable. The k8s
error publication would introduce a new evidence family and has a separate
possible unsynchronized-read concern. Neither was implemented.

Process/cancellation (five): trzsz daemon handoff; three Rekor process-lifetime
signal registrations; a Sponge timeout followed by a longer sleep. No blanket
main-function, daemon-name, or timing exemption was added.

Miscellaneous (16): seven Agola service-completion sends; five concurrent
capture cases; three defer-in-loop cases; one deliberately recovered Plow
channel panic. An earlier producer experiment removed all 26 detected bug
controls and was rejected. Do not retry a blanket complete-worker gate.

Five singleton-loop cases need an explicit policy decision before introducing
loop-count reasoning (four Clusternet and one Paperboy). Approval was asked
but not received. The repository currently forbids that FP-fix approach.
Retirement/demotion of any check also requires user approval.

## Reproduction and practical constraints

Pinned checkouts are `.build/audit-batch{51..55}/checkouts/owner__repo`.
The immutable starting binary is `.build/gohawk-followup78-goroutines-v5`, SHA
`5516cad4c83bffd8dca28713df53f8d3d1a463b838c23d302da9e10ddc257419`.
Every family ledger points to candidate hashes and exact receipt paths. Many
scratch replay scripts and SSA dumps are local-only under `.build`.

Use the canonical direct CLI profile, not a silently narrower check:

```sh
env PROTO_REPORTER=text CGO_ENABLED=0 GOWORK=off GOMAXPROCS=2 \
  GOFLAGS='-mod=readonly -p=2' GOTOOLCHAIN=local \
  /absolute/path/to/immutable-gohawk -enable-all -gohawk-include-tests -json ./package
```

Run candidate repositories only through static analysis—never their tests,
generators, or applications. Keep at most four external scans running across
all workers; use process-group timeouts. Go is already in PATH. Build candidates
at new paths; do not overwrite a binary used by an active replay.

For local validation prefer `make verify`, focused `go test -count=1` (new
testdata files can otherwise be hidden by test caching), race tests, and the
scoped official precision targets. Preserve original labels and all failed
candidate receipts. Commit focused changes and push to the existing upstream;
never stage unrelated work or switch branches in the shared checkout.
