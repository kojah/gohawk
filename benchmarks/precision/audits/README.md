# Precision audits

Latest: [overnight 1,250-repository audit](overnight-2026-09-22.md), covering
batches 51–55, verified corrections and remaining limitations.

Targeted follow-up: [use-after-release promotion](use-after-release-promotion.md)
records the storage-aware check's fixtures and two pinned validation groups.

`500-repository.tsv` is the append-only selection ledger for the 50-batch
`-enable-all` audit. Repository revisions and analyzed modules are pinned so a
finding can be reproduced after the temporary checkout is gone. `reviewed`
means the batch's diagnostics were grouped and inspected for generalizable,
high-confidence false positives; it does not mean every policy finding was
adopted by the external project.

## Batch 1

Ten repositories and eleven modules were scanned. Three recurring evidence
gaps were fixed:

- an immediately invoked closure that registers a nested cleanup defer;
- a launched process waiter that registers `Wait` through a nested defer; and
- direct reaping through `cmd.Process.Wait`.

The fixes remove findings in uzomuzo, feint, and mache. Nearby unclosed HTTP
responses, transactions without rollback, and successful process starts with
no complete reap path remained reportable and are pinned in precision round 5.
Cloudemu produced no findings. Uzomuzo's CGO-only tree-sitter package did not
load under the audit's `CGO_ENABLED=0` environment; its other packages were
still analyzed.

## Batch 2

Ten repositories and twenty-one modules were selected. Twenty modules loaded
under `CGO_ENABLED=0`; TerraTidy's `examples/go-rule` module was blocked by a
missing committed `go.sum` entry.

Four general evidence gaps were fixed:

- send-only and receive-only conversions of the same channel now retain one
  identity;
- a map-range worker launch may be paired with receives bounded by the stable
  length of that same map;
- `testing/synctest.Test` owns goroutines started in its bubble; and
- declarations compiled only from `_test.go` are not treated as production
  APIs.

More involved dynamic-lock, guarded-branch, recursive-worker, custom lifecycle,
and multi-stage resource transfers remain conservative rather than receiving
project-specific exceptions. Nearby caller-channel ownership, goroutine,
resource, and defer-in-loop findings remain reportable in precision round 6.

## Batch 3

Ten repositories and fourteen modules were selected. Eight modules loaded
fully. BlackStork, Codefly, gh-aw-mcpg, and PowerContext produced useful partial
analysis alongside package errors. Dranet's webhook module needed uncommitted
module updates, and its site module contained no Go packages.

Four general evidence gaps were fixed:

- a true `errors.Is` check against a known non-nil filesystem sentinel proves
  that the resource acquisition failed;
- a type implementing `context.Context` may retain the context it delegates to;
- named testing callbacks used only as `func(*testing.T)` or
  `func(*testing.B)` arguments are callback boundaries rather than helpers; and
- returning an aggregate that already contains a goroutine lifecycle owner
  transfers that ownership to the caller.

Dynamic worker counts, channel fields carried through returned work records,
framework lifecycle hooks, cleanup containers, and interprocedural close chains
remain deferred until they have compact general proofs. Precision round 7
retains nearby context, resource, process, goroutine, and defer-in-loop findings.

## Batch 4

Ten repositories and ten root modules were selected. Nine loaded on the host;
WinTUI loaded under its intended Windows target.

Five general evidence gaps were fixed:

- assigning cancellation into an enclosing captured variable transfers the
  obligation to that lifecycle;
- a deferred static helper may invoke an exact bound cleanup callback on every
  normal return;
- `Row` alone is not evidence that a struct is serialized;
- a testing handle used only by a returned callback is not an outer helper
  boundary; and
- a receive-only stop channel may be observed through exact, source-visible
  static helper calls.

Field-sensitive channel ownership, receiver registries, returned cleanup
callbacks, pre-spawn deferred owners, and send-only close ownership remain
deferred. Precision round 8 retains nearby resource, process, testing, context,
and goroutine findings.

## Batch 5

Ten repositories and fourteen modules were selected. Twelve modules loaded.
Mattermost's dependency set did not compile against its selected HTTP/2 API,
and KEDA's root module needed uncommitted module updates; KEDA's Terratest
module loaded normally.

Four general evidence gaps were fixed:

- named methods used only as exact testing callbacks are callback boundaries;
- a wholly unused testing handle cannot affect failure attribution;
- a private static helper may transfer cancellation to a goroutine that invokes
  the exact callback on every return; and
- a deferred function literal may close an exact field projection passed as
  its formal parameter on every return.

Field-sensitive channels, pipe endpoint contracts, context-bound adapters,
name-derived producer documentation, and injective-map determinism remain
deferred until they have compact structural proofs. Precision round 9 retains
nearby testing, cancellation, resource, and goroutine findings.

This evidence also corrected the earlier round-8 classification of
`minimalWebP(*testing.T)`: because the testing handle is wholly unused, calling
`Helper` cannot change failure attribution. The pinned case remains in the
cohort as a false-positive regression rather than being discarded.

## Batch 6

Ten repositories and twelve modules were selected. Nine modules loaded fully.
snclient's buildtools module contained no packages; Linode and itch-setup
produced useful partial analysis while native cryptsetup and GTK packages were
unavailable under `CGO_ENABLED=0`.

Three general evidence boundaries were improved:

- an exported string field populated through exact `encoding/json.Unmarshal`
  is not a source-closed domain;
- exact `testing.T` or `testing.B` cleanup may terminate the sole launched
  lifecycle on the same receiver; and
- a WaitGroup completion signal proves a join only when it settles the worker,
  rather than when `Done` runs before later substantive work.

Cross-type context propagation through `io/fs` adapters remains deferred: the
reviewed family combines copy-style `WithContext`, constructor-bound adapters,
and returned file wrappers, so a sound exemption needs more than interface or
type-name evidence. Precision round 10 retains nearby closed-domain, resource,
and goroutine findings.

## Batch 7

Ten repositories and seventeen modules were selected. Fifteen modules loaded
fully under the host configuration. Windows Exporter loaded under its intended
Windows target, while Beyla's root module produced useful partial analysis
because generated eBPF declarations were unavailable; its tools module loaded
normally.

Four general evidence boundaries were improved:

- external JSON population follows nested aggregates to their reachable fields;
- a `testing.T` cleanup that waits for the exact WaitGroup joins its goroutine;
- deferred functions use the concrete return-slot value on the feasible log-to-return path; and
- production and test variants share one canonical syntax source, avoiding duplicate evaluation-order diagnostics.

Complex Restic and Temporal lifecycle flows, receiver registries, and
cross-method ownership and join chains remain deferred rather than receiving
project-specific exceptions. Precision round 11 retains nearby closed-domain,
evaluation-order, resource, producer, error, and goroutine findings.

## Batch 8

Ten repositories and thirteen modules were selected. Twelve modules contained
Go packages and loaded fully; Zap's assets module contained no Go files. NATS'
optional test dependencies were resolved through a disposable module file so
the pinned checkout remained unchanged while its complete test tree was
analyzed.

Five general evidence boundaries were improved:

- a successful comma-ok assertion on the exact acquisition error proves that
  the acquisition failed;
- `os.IsExist` is recognized as the legacy counterpart of an existing-file
  error check;
- a deferred `WaitGroup.Wait` registered on every path before a spawn joins
  workers whose exact group is settled by terminal `Done` calls;
- ranging an exact channel is an explicit stop lifecycle, just like receiving
  from it directly; and
- an immediately invoked closure may release an exact lock on every normal
  return even when branches precede the release.

Cross-method worker-pool ownership, callback registries, returned server
cleanup closures, and API- or protocol-dependent test goroutine completion
remain deferred rather than receiving name-based exceptions. Precision round
12 retains nearby lock, resource, producer, process, and goroutine findings.

## Batch 9

Ten repositories and twenty-one modules were selected. Twelve modules loaded
fully. CEL's root and security modules, PeerDB's flow module, and OTel Desktop
Viewer produced useful partial analysis because of upstream source, generated
code, or native dependency constraints. Four CEL utility modules did not load,
and Loop's tools module contained no buildable packages.

Four general evidence boundaries were improved:

- constructing a function literal does not execute mutations in its delayed
  body, while directly invoked literals remain part of operand evaluation;
- a static helper may own an exact resource by registering a deferred cleanup
  callback on every normal return;
- a map range directly inside an exact `len(map) == 1` branch has no observable
  iteration-order choice; and
- a finite set of exact bound methods may receive close ownership when every
  function-value reference resolves and every outer caller relinquishes the
  channel.

Exported cross-package channel ownership and returned cleanup closures for
opaque server lifecycles remain deferred because the available local evidence
cannot prove all callers or termination. Orchard's map iteration remains
reportable because it can return a key-dependent validation error before its
later sort. Precision round 13 retains nearby resource, process,
defer-in-loop, determinism, and goroutine findings.

## Batch 10

Ten repositories and thirty modules were selected. Sixteen modules loaded
fully, three contained no Go packages, and the remainder produced useful
partial analysis despite generated assets, native dependencies, target-specific
code, or readonly module metadata gaps.

Two general evidence boundaries were improved:

- lock acquisition and release guarded by the same stable Boolean parameter
  now share one feasible-path condition; and
- a deferred static helper may close an exact field or constant-index
  projection when that projection maps to the helper parameter and cleanup
  occurs on every normal return.

Nested returned process owners, aggregate lifecycle registries, conditional
resource replacement, parameter-sensitive process termination, and temporal
idempotent-cleanup state remain deferred because sound proofs require broader
alias, cross-method, or state-transition evidence. Precision round 14 retains
nearby lock, resource, determinism, and concurrent-capture findings.

## Batch 11

Ten repositories and twenty-seven modules were selected. Twenty-one modules
loaded and scanned fully, five produced useful partial analysis, and gdg's
tools module contained no Go packages. Collector was partial under
`CGO_ENABLED=0` because native PostgreSQL parser APIs were unavailable. Four
Pyroscope example modules were partial because two source files declared
`main`; eight OSV testdata modules loaded fully and produced no findings.

Three general evidence boundaries were improved:

- unexported compiler-initialized `//go:embed []byte` data may reuse the
  package-wide read-only collection proof, including exact
  `net/http.ResponseWriter.Write` consumption;
- an inline cleanup error returned only when an exact prior error is nil
  preserves deliberate prior-error precedence; and
- independent error-producing calls no longer share identity merely because
  they consume the same ordinary payload, while exact `%w` wrapping and
  error-derived observations retain provenance.

The gdg correction removed the reviewed contact-points false positive and
surfaced a separate true positive where `DeleteTeam` logs `err.Error()` and
returns that exact error. Order-insensitive DSN sinks, immutable-after-build
templates, uniqueness-guarded map selection, mutually exclusive prefix
predicates, receiver-managed cancellation and channel lifecycles, returned
goroutine owners, and opaque externally observed joins remain deferred until
they have compact structural proofs. Precision round 15 retains nearby
global-state, closed-domain, resource, error-ownership, and determinism
findings.

A constant-size performance check compared three warmed `-enable-all` runs on
pinned Caddy before and after the code changes. Median wall time changed from
3.445 to 3.425 seconds (-0.6%), while median peak RSS changed from 4,963,904 to
5,255,104 KiB (+5.9%). This fixed cohort is used per batch; the complete
benchmark remains a periodic and release gate.

## Batch 12

Ten repositories and twelve modules were selected. Ten modules loaded and
scanned fully. Watchman produced useful partial analysis across 52 of 54
packages because its native Fyne/OpenGL packages were unavailable under
`CGO_ENABLED=0`; Memefish's docs module contained no Go packages.

Two general evidence boundaries were improved:

- empty, unexported value-receiver methods used only as interface markers no
  longer establish mixed receiver semantics; and
- a reverse lookup over a fresh constant map is order-independent when every
  value is distinct and the loop selects only the matching key.

The fixes removed all 250 Memefish API-shape findings and the injective
ftpserverlib lookup while retaining ftpserverlib's unordered HASH protocol
output. Precision round 16 passed with all four reviewed false positives absent
and all nine retained true positives present.

Opaque assertion-gated cleanup, fresh-lock publication, callback registries,
count-coupled process waits, and dynamically accumulated returned resource maps
remain deferred until they have compact structural proofs. Nearby resource,
lock, process, context, error-classification, defer-in-loop, and determinism
findings remain reportable. The final scans produced 1,614 diagnostics, down
from 1,849 before the Batch 12 changes.

A constant-size performance check compared three warmed `-enable-all` runs on
pinned Caddy at the Batch 11 boundary and after Batch 12. Median wall time
changed from 3.370 to 3.402 seconds (+0.9%), while median peak RSS changed from
5,224,444 to 5,079,464 KiB (-2.8%). The complete cumulative precision suite is
reserved for every fifth batch; Batch 12 replayed only its new cohort and
directly affected ownership repositories.

## Batch 13

Ten repositories and seventeen modules were selected. Cast produced useful
partial analysis because stale tests call a changed constructor signature. OME
required CGO for its root module and Go 1.25 for its scheduler module; three
auxiliary modules contained no Go packages. All other selected modules loaded
and scanned fully. Final scans produced 1,350 diagnostics.

Two bounded evidence corrections were made:

- a deferred callback now proves a goroutine join only when its exact callback
  unconditionally waits for the WaitGroup settled by that worker; the existing
  `sync.OnceFunc` contract covers the wrapped form; and
- error-return refinement now searches the basic block that owns the SSA load,
  avoiding an out-of-bounds access when a reachable return consumes a load from
  a predecessor block.

The first correction removed two Entire CLI goroutine false positives. The
second allowed EcoHub's full `errorownership` scan to complete and retained its
exact log-and-return findings. Precision round 17 passed with both false
positives absent and all four retained true positives present.

Notifiarr's apparent helper joins remain reportable because `WaitGroup.Add`
runs after the worker is launched, allowing `Done` to race ahead of `Add`.
Returned dynamic resource maps, tuple-returned cleanup closures, paired pipe
endpoints, response retry ownership, error-correlated SQL helper cleanup, and
cross-branch acquisition guards remain deferred until they have compact
result-sensitive proofs. Opt-in detached-goroutine and API-shape findings were
sampled but did not expand default correctness policy.

A constant-size performance check compared three warmed `-enable-all` runs on
pinned Caddy before and after Batch 13. Median wall time changed from 3.309 to
3.393 seconds (+2.5%), while median peak RSS changed from 5,038,796 to 5,214,088
KiB (+3.5%). The complete cumulative precision suite remains reserved for
Batch 15; Batch 13 replayed only round 17 and directly affected repositories.

## Batch 14

Ten repositories and eleven modules were selected. Ten modules loaded and
scanned fully. go_proxy_pool's single package could be listed, but its committed
module sums omit required Gin and YAML entries, so the read-only scan produced
no usable diagnostics. Final scans produced 2,421 diagnostics.

Two narrow ownership boundaries were corrected:

- passing the exact cancel function to a source-visible helper launched with
  `go` is now an explicit but ambiguous handoff. Conditional invocation in that
  worker is not proof of release, and ambiguity suppresses the default loss
  diagnostic; and
- returning an exact callback that invokes `Wait` on the WaitGroup settled by
  a worker transfers the join obligation to the caller.

The corrections removed Infercrane's conditional lease-cancellation finding
and Freehire's returned SSE-heartbeat stop finding. Precision round 18 passed
with both false positives absent and all three retained true positives present.

Receiver-held process reaping, interface-dispatched response ownership,
multi-field returned resource aggregates, error-correlated SQL cleanup, and
loop-iteration error identity remain deferred until they have compact
result-sensitive proofs. Mocked HTTP error paths and opt-in policy findings
were sampled without expanding default correctness policy.

A constant-size performance check compared three warmed `-enable-all` runs on
pinned Caddy at the Batch 13 boundary and after Batch 14. Median wall time
changed from 3.174 to 3.049 seconds (-3.9%), while median peak RSS changed from
5,267,632 to 5,016,372 KiB (-4.8%). The complete cumulative precision suite is
scheduled for Batch 15; Batch 14 replayed only round 18 and affected
repositories.

## Batch 15

Ten repositories and fourteen maintained modules were selected. Ten modules
loaded and scanned fully. Gala lacked its Bazel-generated grammar and embedded
standard-library declarations; GGCode's desktop module lacked generated web
assets; Tongstock lacked a replaced sibling module, generated web assets, and
CGO-backed tray symbols; and Pulumi Docker's provider required unapplied
upstream patches and a generated schema. Those four checkouts still produced
useful partial analysis from their independently loadable packages. Final
scans produced 1,541 reliable diagnostics.

Two bounded lifecycle corrections were made:

- exported lifecycle summaries now reuse the exact deferred-callback proof
  when a deferred literal closes a field projected from the summarized
  parameter on every return; and
- a defer that dominates a later lock acquisition and may unlock that exact
  lock makes a missing-release claim uncertain. The defer is checked once at
  acquisition and carried through the existing lock-flow state.

The first correction removed two Qist response-lifetime false positives. The
second removed twelve return-path reports caused by one Telekom guarded
rollback defer. Precision round 19 passed with all three representative false
positives absent and all three nearby true positives present.

GGCode's response cleanup through `safego.Go`, receiver-held resource and
process lifecycles, result-sensitive aggregate ownership, and policy-only API
and determinism findings remain deferred. In particular, the audit did not add
a multi-hop helper/trampoline/callback proof merely to recognize two sites.
Batch 15 found no default goroutine- or cancellation-ownership family that
justified expanding either proof system.

The five-batch cumulative gate replayed rounds 2 through 19. It also reconciled
older labels with the conservative ownership policy introduced before this
batch: thirteen former correctness labels now represent ambiguous handoffs and
are false positives, while one intentionally detached daemon is a true
positive only for the opt-in detached audit. After that review, all 158 false
positives remained absent and all 198 true positives remained present.

Five-run performance comparisons used the Batch 14 boundary and the final
Batch 15 code revision on the same host. Pinned Caddy median wall time changed
from 3.035 to 3.172 seconds (+4.5%) and peak RSS from 4,964,780 to 5,160,124
KiB (+3.9%). The periodic pinned Moby checkpoint changed from 7.106 to 7.426
seconds (+4.5%) and from 10,631,592 to 10,649,680 KiB (+0.2%). Repeated runs
showed comparable host variance in both directions; focused Caddy runs for the
changed resource and lock analyzers were faster than the Batch 14 boundary.

## Batch 16

Ten repositories and fourteen maintained modules loaded and scanned fully.
Final scans produced 1,883 diagnostics after two bounded corrections:

- taking an exact identifier's address in an earlier operand establishes stable
  storage identity, not a stale value snapshot. Mixed address and value
  operands remain reportable; and
- an exact Testify `assert.NoError` condition tied by SSA identity to the
  acquisition error establishes that the true successor is the success path.

The first correction removed 46 Open Next Router evaluation-order false
positives while retaining a nearby Redis stale-value diagnostic. The second
removed 22 Kong resource-lifetime false positives while retaining unrelated
resource and goroutine findings. Precision round 20 passed with all three
representative false positives absent and all three retained true positives
present.

Five Duckgres missing-release findings remain deferred. Three require symbolic
reasoning about Boolean phi values, while two pass returned cleanup closures
through a nested `sync.Once.Do` callback. Adding either multi-hop proof family
for these isolated findings would make the default lock proof substantially
more open-ended. A post-start process failure in the same repository remains a
confirmed missing-wait diagnostic. Opt-in detached-goroutine and policy findings
were sampled without expanding default correctness proofs.

A constant-size performance check compared three warmed `-enable-all` runs on
pinned Caddy at the Batch 15 boundary and after Batch 16. Median wall time
changed from 3.789 to 3.254 seconds (-14.1%), while median peak RSS changed from
4,955,976 to 4,930,464 KiB (-0.5%). The cumulative precision suite remains
reserved for Batch 20; Batch 16 replayed only round 20 and affected
repositories.

## Batch 17

Ten repositories and sixty-three maintained modules loaded and scanned fully.
WSO2 Gateway Controllers contributed fifty-four small policy modules; the
remaining repositories each contributed one root module. Final scans produced
867 diagnostics after two bounded corrections.

- A guarded resource acquisition may merge its exact resource and error with
  nil before rechecking the same guard. One local structured proof now follows
  only that direct acyclic diamond, prunes the impossible non-acquisition
  branch, and accepts cleanup through the exact resource phi. Other non-nil
  edges and loop-carried phis remain unproven.
- An unexported `strings.Replacer` constructed by the exact standard-library
  function is accepted only when every use is a direct exact `Replace` or
  `WriteString` call. Reassignment, addressing, aliasing, passing, returning,
  method values, alternate constructors, and lookalikes remain reportable.

The first correction removed one CortexDB resource-lifetime false positive.
The second removed one Aerospike global-state false positive. Precision round
21 passed with both representative false positives absent and all four nearby
true positives present, including CortexDB's discarded HTTP response and
unwaited process and Aerospike's mutable slice and map-order output.

An all-callers sorting proof for Docket's private `mapKeys` helper remains
deferred: one policy finding did not justify a new package-wide caller analysis
and its large negative-fixture surface. Kubewarden's apparent deletion-set
case remains reportable because the producer logs malformed entries in map
order before returning. Read-only collections passed through helpers and
opt-in component-lifetime goroutines were also sampled without expanding
default correctness proofs. No default goroutine- or cancellation-ownership
false-positive family appeared.

A constant-size performance check compared three warmed `-enable-all` runs on
pinned Caddy at the Batch 16 boundary and after Batch 17. Median wall time
changed from 3.187 to 3.176 seconds (-0.4%), while median peak RSS changed from
4,929,632 to 4,925,924 KiB (-0.1%). The cumulative precision suite remains
reserved for Batch 20; Batch 17 replayed only round 21 and affected
repositories.

## Batch 18

Ten repositories and twenty-four maintained modules were reviewed. Twenty-
three modules loaded and scanned fully. Forge ADE produced a useful partial
scan because its pinned revision omits the embedded `frontend/dist` tree and
the available JavaScript lockfile could not reproduce it. Final scans produced
1,257 diagnostics after two bounded ownership corrections.

- An optional worker created with one exact local stop channel and WaitGroup is
  accepted when the launch proves the channel non-nil and a later exact nil
  guard dominates both `close` of that channel and `Wait` on that group. The
  proof rejects rebinding, related sentinels, different groups, and general
  branch correlation.
- An imported cleanup fact may settle an acquired resource through one exact
  stable projection only when the original owner has not been mutated or
  allowed to escape before the call. Conditional facts, sibling values,
  reassigned fields, phis, and opaque aliases remain unproven.

The first correction removed one Rainier goroutine-ownership false positive.
The second removed two IBM resource-lifetime false positives. Precision round
22 passed with all three representative false positives absent and all three
nearby true positives present, including Rainier's skipped-defer exits and
IBM's unreleased similarity response.

Several broader candidates remain deliberately deferred. Retrogolib's custom
assertion helper would require interprocedural dominance proofs across a
structural testing handle. Pocket ID's sorted result passes through two
transforming helpers and would require a recursive ordered-return summary.
Rainier's package-wide child reaper and receiver-owned resource map likewise
lack compact local ownership proofs. None justified a name exemption or a new
open-ended proof family. Opt-in policy and detached-worker findings were
sampled without changing their default status.

A constant-size performance check compared three warmed `-enable-all` runs on
pinned Caddy at the Batch 17 boundary and after Batch 18. Median wall time
changed from 3.373 to 3.318 seconds (-1.6%), while median peak RSS changed from
4,910,616 to 5,015,196 KiB (+2.1%). The cumulative precision suite remains
reserved for Batch 20; Batch 18 replayed only round 22 and affected
repositories.

## Batch 19

Ten repositories and forty-two maintained modules were reviewed. Forty modules
loaded and scanned fully; New Relic's tools module intentionally contains no Go
packages, while one rslint shim could not verify a missing upstream checksum in
read-only module mode. Final scans produced 2,624 unique diagnostics. No
analyzer change met the batch's boundedness threshold.

Several candidates remain deliberately deferred. Rslint's worker is joined by
a callback selected on the launch branch and invoked through `sync.Once.Do`,
but suppressing it safely would require temporal Once state plus writes through
captured variables. Proving New Relic's in-memory compression error path
infeasible would require a four-constructor catalog of compositional standard-
library I/O behavior. A duplicate NSX capture finding appears only in
the per-analysis-action JSON tree; ordinary text already deduplicates it, and
the audit counts it once. Rslint's same-key map projection can emit failures to
stderr in map order, while wp2hugo's apparently commutative aggregation can
return the first map-selected URL error, so both remain credible determinism
findings. No name exceptions or open-ended proof families were added.

Batch 19 made no analyzer or runtime changes, so its Caddy binary is identical
to the Batch 18 boundary and no new performance comparison was necessary. The
cumulative precision replay and larger benchmark remain scheduled for Batch
20.

## Batch 20

Ten repositories and eighteen modules were selected. Twelve modules loaded
and scanned fully. Gouncer's five modules require Go 1.27.1 and did not load
under the audit's pinned local toolchain, so that repository contributed no
findings; the Temporal worker controller's `internal/tests` module produced a
useful partial scan because a pinned controller-runtime dependency no longer
type-checks against its own cache package. Final scans produced 919 unique diagnostics after three shared
value-mechanics corrections.

- External ownership now sees through a type assertion, so a channel read
  from an asserted event parameter belongs to the event's producer. This
  removed one YuniKorn goroutine-ownership false positive in a test mock.
- A returned closure that captured the owner, and a by-value copy of the
  owner, now carry it. Returning `ephemeral(*cmd)` transfers a started
  process, which removed one miniredis process-ownership false positive.
- A source-visible callee that returns the owner on every normal path, with
  a result the caller lets escape, transfers the obligation. Lifecycle facts
  summarize only exported functions, so this local proof covers unexported
  helpers such as statedb's iterator wrapper. It removed two resource-lifetime
  false positives.

Precision round 23 passed with all four false positives absent and all six
nearby true positives present: response bodies leaked on unauthorized and
error paths in Spegel and datarhei, a temporary file leaked on a write error
in datarhei, and gzip readers and writers leaked on error paths in git-pkgs.

Two candidates remain deliberately deferred. YuniKorn stores a gzip writer in
a response wrapper whose deferred method closes it only when compression was
chosen, which would need a proof over a method on a local aggregate. An sdns
ticker feeds a `for range` loop that never exits, so its missing `Stop` is
unreachable rather than leaked; proving that would need a never-closed-channel
contract for `time.Ticker`. Opt-in detached-goroutine, global-state, API-shape,
and test-policy findings were sampled without expanding default correctness
policy.

The replay harness now warns when a scan exits abnormally or returns no
payload. Under an eight-way parallel replay, several large repositories had
been killed silently and their reviewed true positives reported as lost; the
sequential replay confirmed every label.

This is the fifth-batch milestone. The cumulative gate replayed rounds 2
through 23: all 170 false positives remained absent and all 214 true positives
remained present. Batch 20 also followed the goroutineownership rebuild, which
replaced the analyzer's proof ladder with one instruction classifier and one
flow query; that change was replayed against the same cumulative gate before
this batch began.

Three-run performance comparisons used the Batch 19 boundary and the final
Batch 20 code revision on the same host. Pinned Caddy median wall time changed
from 3.388 to 3.206 seconds (-5.4%) and peak RSS from 4,879,300 to
5,065,572 KiB (+3.8%). The periodic pinned Moby checkpoint changed from
7.187 to 7.067 seconds (-1.7%) and from 10,174,812 to 10,216,948 KiB
(+0.4%). The interval includes the goroutineownership rebuild and the
ssaflow completion collapse as well as this batch's corrections.

## Batch 21

Ten repositories and forty-two modules were selected; the Binance connector
alone contributed twenty-nine per-product client modules. Thirty-five modules
loaded and scanned fully. Blockbook, kuberpult's root module, pkimetal, and the
Mattermost agents plugin produced useful partial analysis because of a native
ZeroMQ binding, an unpublished sibling module, Linux-only build constraints,
and a pinned gRPC dependency that no longer type-checks; the Binance and
Interuss example modules contained no analyzable packages. Final scans
produced 5,722 unique diagnostics, dominated by API-shape and
context-policy findings in Binance's generated client code.

Two bounded corrections were made:

- a closure passed to `sync.WaitGroup.Go` is launched exactly like a go
  statement, so a resource released on every return of that closure is
  settled by it; and
- a cancel function stored into a local that another closure captures is an
  ambiguous handoff rather than private retention, because a deferred guard
  may release it on every return through code the proof does not follow.

The corrections removed one Safebucket ticker false positive and one
Safebucket cancellation false positive. Precision round 24 passed with both
false positives absent and all five nearby true positives present: two
decoder files left open in Blockbook's build tool, an ffmpeg process skipped
on transcription errors in the Mattermost plugin, an unread response body in
a kuberpult health test, and a per-iteration close deferred across a loop in
llm-d's metrics test. No default goroutine-ownership false positive appeared.

Kuberpult's conditionally cancelled test context, gob's logged-and-returned
errors, and llm-d's non-atomic `sync.Map` load-and-delete remain reportable
as written. Opt-in detached-goroutine, API-shape, and context-policy findings
were sampled without expanding default correctness policy.

A five-run performance check compared warmed `-enable-all` runs on pinned
Caddy at the Batch 20 boundary and after Batch 21. Median wall time changed
from 3.824 to 3.385 seconds (-11.5%), while median peak RSS changed from
4,875,948 to 4,892,836 KiB (+0.3%). An earlier three-run comparison taken
immediately after a full rescan showed an eighteen percent slowdown that the
idle-host re-measurement did not reproduce. Batch 21 also replayed rounds 2
through 24 in full, and all 170 false positives remained absent and all 214
true positives remained present; ordinary batches from Batch 22 on replay only
the new cohort and the rounds whose labels involve the analyzers the batch
changed, with the cumulative suite reserved for Batch 25.

## Batch 22

Ten repositories and twenty modules were selected. Sixteen modules loaded
and scanned fully. Sync Gateway's root module produced useful partial analysis
because its pinned Rosmar dependency no longer type-checks against SQLite
bindings; taskyou's `ty-qmd` extension needed uncommitted module updates; and
the openperouter website and Sync Gateway tooling modules contained no Go
packages. Final scans produced 1,784 unique diagnostics. No analyzer change
met the batch's boundedness threshold, and no default false positive was
found.

The default correctness findings were confirmed on inspection. Openperouter's
host controller closes a completion channel in its static reconciler worker
and waits on it before starting the Kubernetes reconciler, but returns early
without that wait when the API never becomes reachable, which is the first
real unjoined-goroutine report since the classifier rebuild. GoModel's
benchmark tool reads and never closes response bodies; jx-gitops passes freshly
opened files to a helper that closes them only after a successful encode and
sync, creates release-notes and temporary files it never closes, and leaves a
test temporary file open; Ramen, openperouter, and gowaves leak gzip readers,
temporary files, profile files, and block files on error paths; and a
crossplane provider mutates a captured error from goroutines launched in a
loop. Taskyou's seven unwaited `open` and daemon launches are deliberate
fire-and-forget processes that the process-ownership policy still reports.

Opt-in exit-policy, error-ownership, API-shape, and global-state findings were
sampled without expanding default correctness policy. Batch 22 made no
analyzer or runtime changes, so its binary is identical to the Batch 21
boundary and no performance comparison was necessary.

## Batch 23

Ten repositories and twenty modules were selected. Nineteen modules loaded
and scanned fully; Kolide's launcher produced useful partial analysis because
its pinned fscrypt dependency needs CGO. Final scans produced 1,248
unique diagnostics after five bounded corrections, all of them shared value
or contract mechanics rather than new proof families:

- a source-visible helper that registers `testing.Cleanup` calling the
  lifecycle method on its exact argument owns that argument, since Cleanup
  runs after the test regardless of how the helper returns;
- `os.IsPermission` and `os.IsTimeout` join `os.IsNotExist` as documented
  predicates that report false for a nil error;
- a fatal `require.Error`, or `require.NotNil` applied to the error, stops the
  test unless the acquisition failed, for any acquisition rather than only
  `net/http`;
- a struct literal returned by value carries whatever the local that
  assembled it holds, including a method value bound to the resource; and
- a channel read through one slice element is matched against channels stored
  through any element of the same slice, so buffered per-producer results do
  not become a proven skipped join.

The corrections removed seven filesql fixture-file findings, two filesql and
two Kolide test findings, one warp reader wrapper finding, and one Shopware
goroutine finding. Precision round 25 passed with all six representative false
positives absent and all eight nearby true positives present: Kolide's ticker
created before a failing constructor step, its transaction abandoned by a
size guard, and its file created before a failing chmod; warp's ticker
leaked on the stability exit and its CSV loader that skips its join on a
parse error; and Shopware's mis-checked scan error and unclosed icon file.

Several findings remain reportable as written: gzip and zlib writers left
open on write errors, response bodies read but never closed, temporary files
created and only removed, a transaction begun inside a goroutine and never
rolled back on the unexpected-success path, and the `open` and `xdg-open`
launches that warp and resterm deliberately never wait for. Opt-in
exit-policy, global-state, and API-shape findings were sampled without
expanding default correctness policy.

A five-run performance check compared warmed `-enable-all` runs on pinned
Caddy at the Batch 22 boundary and after Batch 23. Median wall time changed
from 3.265 to 3.366 seconds (+3.1%), while median peak RSS changed from
5,138,656 to 4,969,540 KiB (-3.3%). Batch 23 replayed round 25 and every
round carrying labels for the changed analyzers, which is all of them; all 178
false positives remained absent and all 227 true positives remained present.
The cumulative suite and larger benchmark remain scheduled for Batch 25.

## Batch 24

Ten repositories and twenty-seven modules were selected. Twenty-four modules
loaded and scanned fully. The Arduino flasher and Probo root modules produced
useful partial analysis because a pinned gRPC dependency and an embedded
frontend tree were unavailable, and Casbin Gateway's desktop module is
excluded by build constraints on this host. Final scans produced
2,169 unique diagnostics after two bounded corrections:

- a worker that sends on or closes an element selected from a captured
  aggregate signals through that aggregate. The parent joins by receiving
  from any part of it and transfers it by returning or handing it on, so a
  readiness-only `Done` before the shard work is no longer a broken join; and
- returning the exact `os.Process` projected from a started command hands the
  caller a handle it can wait on, which transfers the reap obligation like
  returning the command itself.

The first correction removed one Nacos goroutine-ownership false positive in
its concurrent map snapshot. The second removed one Casbin Gateway
process-ownership false positive in its daemon launcher. Precision round 26
passed with both false positives absent and all six nearby true positives
present: Auth0's login ticker never stopped, its quickstart temporary file
left open on a copy error, test temporary files created and only removed,
and Chatto's compression writer leaked on a write error and a discarded test
response.

Eight `open`, `xdg-open`, and `rundll32` browser launches in Probo and two
development servers started by the Auth0 CLI remain reportable: the process
policy requires a wait or an explicit release, and a name-based exemption for
launcher commands is not a structural contract. Opt-in API-shape,
global-state, and wire-policy findings, most of them in Probo's and
Gouroboros' generated code, were sampled without expanding default policy.

A five-run performance check compared warmed `-enable-all` runs on pinned
Caddy at the Batch 23 boundary and after Batch 24. Median wall time changed
from 3.539 to 3.169 seconds (-10.5%), while median peak RSS changed from
4,985,968 to 4,945,412 KiB (-0.8%). Batch 24 replayed round 26 and the
twenty-three rounds carrying goroutine- or process-ownership labels; all 174
false positives remained absent and all 219 true positives remained present
after one Kong scan killed by memory pressure under a three-way replay was
rerun alone. The cumulative suite and larger benchmark are scheduled for
Batch 25.

## Batch 25

Ten repositories and eighteen modules were selected. Seventeen modules loaded
and scanned fully; Fabric Smart Client's tools module contained no Go
packages. Final scans produced 1,850 unique diagnostics after three
bounded corrections, each a reuse of an existing proof at one more shape:

- a deferred call to an imported helper whose lifecycle summary says it
  invokes its callback parameter on every return settles the cleanup method
  value bound to the exact resource, the same evidence the local
  deferred-helper proof already accepted for source-visible helpers;
- a source-visible method that appends the argument to a receiver field
  transfers it even though `append` packs its variadic arguments into an
  array before the call, which plain operand derivation cannot follow; and
- a record that holds the resource stored into a receiver-owned map transfers
  it exactly as storing the resource itself would.

The corrections removed four Fabric Smart Client findings, three deferred
`utils.IgnoreErrorFunc(rows.Close)` calls and one profile closer, and two
grix log-file findings. Precision round 27 passed with all six false positives
absent and all eight nearby true positives present: Fabric's CPU-profile file
leaked when profiling fails to start, its gRPC stream test that skips its own
join on a send error, its web client that returns without closing a non-OK
response, and a test transaction never rolled back; Syswarden's discarded
webhook response and never-waited log tail; an Auth0 test response discarded
after the request; and a Scrutineer test that mutates captured flags from
concurrent goroutines.

Several findings remain reportable or deferred as written. NVIDIA's archive
writer is closed through a function-typed dependency field, which is
indistinguishable from any other opaque use of the writer. Syswarden's
firewall guards return with a mutex deliberately held for a paired release
function, and Fabric orders two locks by pointer address; both are lock
handoff idioms that would need a returned-guard contract or an
address-comparison proof. Fabric's readiness `Done` before subscriber work and
its polling of a channel closed through a method remain a policy report.
Opt-in global-state, API-shape, and detached-goroutine findings were sampled
without expanding default policy.

This is the fifth-batch milestone. The cumulative gate replayed rounds 2
through 27 two at a time, the parallelism the host's memory allows for the
largest Kubernetes checkouts: all 186 false positives remained absent and all
241 true positives remained present.

Five-run Caddy and three-run Moby comparisons used the Batch 24 boundary and
the final Batch 25 code revision on the same host. Pinned Caddy median wall
time changed from 3.402 to 3.477 seconds (+2.2%) and peak RSS from
5,127,312 to 5,036,288 KiB (-1.8%). The periodic pinned Moby checkpoint
changed from 7.865 to 7.475 seconds (-5.0%) and from 10,340,972 to
10,280,640 KiB (-0.6%). Run-to-run variance on this host remains wider than
any of these deltas.

## Batch 26

Ten repositories and seventeen modules were selected. Fifteen modules loaded
and scanned fully. Maintenant produced useful partial analysis because its
pinned revision omits an embedded asset tree, and qdrant's migration tool
depends on ONNX runtime bindings that are excluded by build constraints on
this host. Final scans produced 1,193 unique diagnostics after two
bounded corrections:

- a producer goroutine whose completion channel is captured or received by
  another goroutine launched anywhere in the same function hands its
  completion to those workers. Worker pools launch their consumers in a
  loop, so the existing dominating-launch rule could not credit them, and the
  parent never established a receive of its own to skip; and
- a captured mutex now carries a closure-scoped identity built from its
  free-variable name, so two captured owners of one type are distinct locks
  inside a closure rather than one apparent recursive acquisition.

The first correction removed three Grafana alerting-generator producer
findings and one NetBox SNMP probe finding. The second removed two Trickster
integration-test findings. Precision round 28 passed with all five
representative false positives absent and all seven nearby true positives
present: a traQ test repository that returns with its tag lock held, a Topaz
manifest file never closed, a Grafana WeCom response leaked on a non-2xx
status, a Grafana worker error assigned from concurrent goroutines, two
Trickster test responses and readers never closed, and an iprange pin closed
only after its loop.

Trickster's daemon unlocks through a deferred closure that consults a
Boolean set later in the function, which remains deferred pending symbolic
Boolean reasoning, as with Duckgres in Batch 16. Opt-in global-state,
API-shape, wire-policy, and detached-goroutine findings were sampled without
expanding default policy.

Per-batch and fifth-batch performance comparisons were discontinued from
Batch 26 onward: run-to-run variance on the audit host had exceeded every
delta recorded since Batch 11, so the measurements were not informing
decisions. The dogfooding benchmark remains available for release gates.
Batch 26 replayed round 28 and the twenty-three rounds carrying goroutine- or
lock-order labels.

## Batch 27

Ten repositories and fifteen modules were selected. Thirteen modules loaded
and scanned fully. Gocoin's root module produced useful partial analysis
because its pinned revision references an unpublished sibling package, and a
Duragraph example module could not build against its own SDK. Final scans
produced 1,534 unique diagnostics after two bounded corrections:

- a named helper that receives the cleanup-bearing projection of a resource,
  such as a response body, and calls the lifecycle method on exactly that
  parameter before every return settles the resource whether it is launched
  with `go` or called directly. Function literals keep their existing
  conservative boundary; and
- a WaitGroup join guarded by a local Boolean that the function assigns a
  constant alongside the launch, such as `started = true` before spawning and
  `if started { wg.Wait() }` after, correlates the guard with the launch in a
  way the proof does not model, so the worker is unknown rather than
  reported.

The first correction removed one Duragraph streaming-body finding and two
Kubestellar dashboard findings. The second removed one Soperator collector
finding and one gocoin script-verification finding. Precision round 29 passed
with all five false positives absent and all seven nearby true positives
present: two Kubestellar token checks that leak the response on a non-200
status because the status test shares the error branch, a gzip writer left
open on a write error, a gocoin short-ID loop that returns with the
transaction mutex held, and three gocoin block, wallet, and UTXO fetches that
leak on non-200 or read paths.

Kubestellar's test that starts helper processes in one loop and waits for
them in a later loop, and its descriptor-gauge test that opens files into a
slice, remain reportable pending count-matched process and resource joins.
Its victim processes started and only killed, and Nezha's intentionally
orphaned test helpers, remain policy reports. Opt-in global-state, API-shape,
determinism, and detached-goroutine findings were sampled without expanding
default policy. Batch 27 replayed round 29 and every earlier round.

## Batch 28

Ten repositories and eighteen modules were selected. Seventeen modules loaded
and scanned; cb-spider, LiveReview, and operator-controller produced useful
partial analysis around packages whose dependencies exclude the host
platform, and one operator-controller tool directory holds no packages. Final
scans produced 2,712 unique diagnostics after the completion-engine
unification and five bounded corrections:

- a deferred callee that releases the resource on some path leaves the
  release data-dependent, so resourcelifetime asks only whether the defer
  may release; the transaction idiom that rolls back unless a committed flag
  was set no longer reports, and three conditional-defer fixtures became
  accepted forms;
- a captured variable resolves to its latest dominating store, so a rows
  variable re-queried before its deferred Close still maps to that Close,
  while a variable declared inside a retry loop is a fresh cell each
  iteration rather than a reassignment;
- a select statement that offers a tracked value on a send case is an opaque
  handoff, as a plain send already was;
- a receive from any element of the slice a signal element was loaded from
  counts as a join of that signal, which makes a drain loop over a locally
  built channel slice an unproven counted join; and
- an integer counter stepped beside the launch and compared in a guard, such
  as `started++` before `go` and `if started == 0 { return }` or a receive
  loop bounded by the count, correlates the guard with the launch the way a
  Boolean flag does.

The corrections removed four transaction rollbacks in LiveReview and pad, one
sidecar re-query, one sidecar launched waiter, two cb-spider fan-out joins,
and one rules_img prefetch handoff. Precision round 30 labels those nine
false positives and thirteen nearby true positives: a discarded
`WithTimeout` cancel that cb-spider repeats in thirty-three connectors, a
celestials cancel never deferred, a dispatcher that returns with its mutex
held, two rules_img archive copies that never close the destination, a gzip
reader and two response bodies never closed, an unstopped hourly ticker, an
unbuffered result channel whose sender blocks after the receiver leaves, a
shared error written by parallel goroutines, a defer inside a pod-log loop,
and a server launcher that leaks the child on its PID-file error path.

rules_img's files appended to a local closer slice that a deferred loop
closes, and its flag value appended through a pointer receiver, remain
reportable pending a slice-drain release proof. Nine fire-and-forget browser
launches remain policy reports. Opt-in global-state, API-shape, and
detached-goroutine findings were sampled without expanding default policy.
Batch 28 replayed round 30 and rounds 3, 23, and 29.

## Batch 29

Ten repositories and fourteen modules were selected. Twelve modules loaded
and scanned fully; Ignition produced useful partial analysis around a package
that excludes the host platform, and one gvisor-tap-vsock tools directory
imports a program rather than a package. Final scans produced 751 unique
diagnostics after two bounded corrections:

- lifecycle summaries gain a Retained mask, over-approximate on the retaining
  side, and are exported even when empty so an importer can tell a callee
  proven to do nothing from one never summarized. A literal that releases the
  resource, handed to an opaque callee that retains it, transfers the release
  to that callee's schedule; a callee summarized as dropping the callback,
  such as one that invokes it only under a condition, leaves the obligation
  open; and
- a deferred capture also resolves to a store of the target itself on the
  path to the defer, so a response assigned in either branch of an if/else,
  one of them inside a retry callback, still maps to its deferred close.

The first correction removed one gvproxy log-file finding whose close is
registered with logrus's exit handlers. The second removed two traefikoidc
response findings. Precision round 31 labels those three false positives and
fifteen nearby true positives: a PID file never closed, a stream response
read before its deferred close, an uncompressed image writer never closed,
two gzip writers left open on write errors, a partition tool that returns
before waiting on read errors, a cluster dump file never closed, a retry
timer never stopped, a temporary file leaked on a write error, two responses
that return on status checks before their deferred close, two return
statements whose later operand reassigns the earlier value, a defer inside a
form-file loop, a profile file leaked when profiling fails to start, and a
map written by parallel snapshot workers.

A stdio dialer that stores the process's Kill in a returned connection, and a
keep-alive ticker inside an endless loop, remain reportable. Opt-in
global-state, API-shape, and detached-goroutine findings were sampled without
expanding default policy. Batch 29 replayed round 31 and rounds 23 and 30.

## Batch 30

Ten repositories and sixteen modules were selected. Twelve modules loaded
and scanned fully; arc, p4prometheus, and pyscn produced useful partial
analysis around dependencies that exclude the host platform, and one zot
example module points at a replacement directory that does not exist here. Final
scans produced 1,675 unique diagnostics after two bounded
corrections:

- a callback bound to the lifecycle method, handed to a helper, completes
  when the helper invokes it inside a nested launch as well as directly, and
  a callback spilled to a cell is recognized through its load; and
- lockorder no longer exempts functions by a lock or unlock name prefix.
  Instead, a function whose every successful return that an acquisition
  dominates still holds the lock has the contract of acquiring for its
  caller, while a function that forgets one unlock or acquires only
  conditionally remains reportable.

The first correction removed one crabbox SSH forwarder whose cmd.Wait runs
on a helper's waiter goroutine. The second removed two crabbox
beginOperation findings paired with endOperation. Precision round 32 labels
those three false positives and nine nearby true positives: a gzip writer
whose close is skipped by short-circuit evaluation, a gzip reader left open on
a read error, two gzip writers left open on write errors, an archive reader
never closed, an update ticker never stopped, an RPC client that never waits
on its child, and two defers inside retry loops.

zot's detached bot that kills the child on a PID-file error without waiting,
and micro/mu's news handler that re-acquires a read lock, remain
reportable. Opt-in global-state, API-shape, and detached-goroutine findings
were sampled without expanding default policy. Round 32 was confirmed by
rescanning the cohort; the cumulative milestone replay was deferred.

## Batch 31

Ten repositories and fifty-four modules were selected, most of them gofiber
storage drivers. Forty-eight modules loaded and scanned fully; doco-cd,
buildpacks, and multigres produced useful partial analysis around
dependencies that exclude the host platform, and three example directories
hold no buildable package. Final scans produced 1,214 unique diagnostics
after two bounded corrections:

- goroutineownership counts a literal handed to a call, such as an errgroup
  worker, as a consumer of a tracked signal, so a producer whose jobs channel
  those workers drain is unknown rather than reported; and
- lockorder no longer reports a recursive acquisition when the receiver is
  selected by the loop iteration, since each iteration locks a different
  mutex; the same receiver field locked on every iteration stays reportable.

The first correction removed one Iceberg manifest producer. The second
removed two multigres key-mutex loops. Precision round 33 labels those three
false positives and nine nearby true positives: an nginx config file
leaked when its template fails, a control file never closed, a response body
never closed, three more responses leaked on status checks or discarded, a
retry timer never stopped, a temporary file never closed, and a defer inside
a writer loop. Labels inside nested modules beyond the replay's module budget
were dropped as unverifiable.

A goyacc reader wrapper that never closes its file, a viper persistence timer
captured by a closure, and a multigres watchdog started to outlive its
parent remain reportable or unlabeled. Opt-in global-state, API-shape, and
detached-goroutine findings were sampled without expanding default policy.

## Batch 32

Ten repositories and thirty-one modules were selected, many of them
OpenTelemetry example services. Twenty-six modules loaded and scanned fully;
tootik and goapi produced useful partial analysis around dependencies that
exclude the host platform, and three directories hold no buildable package.
Final scans produced 1,132 unique diagnostics after one bounded
correction:

- evalorder treats an address taken inside an earlier operand, such as the
  `&res` handed to each command constructor in a cobra `AddCommand` list, as
  storage identity rather than an evaluated value, so a later constructor
  that mutates the state through the same address cannot stale it. An
  earlier operand that copies the value stays reportable.

The correction removed fifteen enclave command-tree findings. Precision
round 34 labels two of them and two nearby true positives: a generic reader
that returns its zero value before decoding into it, and a gzip writer left
open on a write error. Two fire-and-forget launchers remain policy reports.
Opt-in global-state, API-shape, and detached-goroutine findings were sampled
without expanding default policy.

## Batch 33

Ten repositories and fourteen modules were selected. Eleven modules loaded
and scanned fully; ally-agent produced useful partial analysis around a
dependency that excludes the host platform, and two ezauth example
directories hold no buildable package. Final scans produced 383 unique
diagnostics and needed no correction.
Precision round 35 pins four true positives: a ticker never stopped inside a
dispatch goroutine, a prepared statement never closed, and two row sets
leaked on scan errors.

A grant query that returns early through an error-classifying helper before
its deferred close, and a jq pipeline waited on under a nil check of its own
handle variable, remain reportable pending nil-correlation evidence. Opt-in
global-state, API-shape, and detached-goroutine findings were sampled without
expanding default policy.

## Batch 34

Ten repositories and sixty-nine modules were selected, most of them example
programs. Sixty modules loaded and scanned fully; ainovel-cli produced useful
partial analysis around a dependency that excludes the host platform, the
ROCm device plugin could not build its driver bindings here, and seven
example directories hold no buildable package. Final scans produced 399 unique diagnostics after one bounded
correction:

- lifecycle summaries gain a strict Stored mask beside the loose Retained
  one: positive structural evidence that a callee keeps a parameter in a
  global, a field, a map, a channel, an append, or an escaping literal, with
  same-package helper bodies followed rather than assumed and returns left to
  the returned-owner summaries. resourcelifetime treats a resource passed to
  a callee summarized as storing it outside its returned value as
  transferred, so a file installed with `log.SetOutput` in an `init` is
  owned by the logger from then on, while a copy helper that only reads
  through the file leaves the obligation in place.

The correction removed two charmbracelet example log files. Precision round
36 labels both and one nearby true positive, a defer inside a transport retry
loop. A request dump file stored through an accessor on the receiver, a
critical section released by a helper that reports whether it unlocked, and
a ripgrep pipeline not waited on scanner errors remain reportable. Opt-in
global-state, API-shape, and detached-goroutine findings were sampled without
expanding default policy.

## Batch 35

Ten repositories and eighteen modules were selected. Thirteen modules loaded
and scanned fully; flagd-proxy and dbtrail produced useful partial analysis
around dependencies that exclude the host platform, and three directories
hold no buildable package. Final scans produced 589 unique diagnostics after two bounded
corrections:

- the non-nil assumption that summaries and completion proofs make about a
  parameter extends to a nil-comparable field loaded from it, such as
  `resp.Body`: when the field is nil there is nothing to release, so a helper
  that closes the body under `resp != nil && resp.Body != nil` is summarized
  as closing it; and
- a captured variable that only ever holds the acquired value or nil, cleared
  after a successful settlement and checked by the deferred literal, still
  reaches the value on every path that did not settle it, so the deferred
  literal may release it.

The first correction removed forty-one autobrr findings that all defer one
shared drain helper. The second removed one witness transaction rollback.
Precision round 37 labels five of those false positives and four nearby true
positives: an SBOM output file never closed, a gzip reader never closed, a
file leaked when its Stat fails, and a response leaked on a method check
before its deferred close.

Two terraform log files opened into the same named result, the second
overwriting the first, a provider wait group whose early returns skip its
Wait, and a helper that reads a log file into a returned writer remain
reportable. Opt-in global-state, API-shape, and detached-goroutine findings
were sampled without expanding default policy.

## Batch 36

Ten repositories and sixteen modules were selected. Thirteen modules loaded
and scanned fully; deck produced useful partial analysis around a dependency
that excludes the host platform, and two lws tool directories hold no
buildable package. Final scans produced 557 unique diagnostics and needed no correction.
Ninety of them are one versitygw integration helper that reads a response
body and returns before closing it when the read fails, so every caller
leaks on that path; precision round 38 pins one of those callers together
with a discarded webhook response, a ticker whose channel is taken without
keeping the ticker, a discarded timeout cancel, and a return that decodes
into a value after copying it.

A gateway root directory handle stored into the returned backend remains
reportable pending field-level ownership of that struct. Opt-in
global-state, API-shape, and detached-goroutine findings were sampled without
expanding default policy.

## Batch 37

Ten repositories and twenty modules were selected. Seventeen modules loaded
and scanned fully; OpenLinkHub produced useful partial analysis around a
dependency that excludes the host platform, and two directories hold no
buildable package.
Final scans produced 908 unique diagnostics and needed no correction.
Precision round 39 pins eight true positives: two returns that hold a named
or package lock, a directory handle and a destination file leaked on read
and copy errors, a gzip reader never closed, a file opened only to test
existence, a temporary update file leaked when its source fails to open, and
a null device never closed.

A destination file closed through a hand-written once wrapper deferred by
its returned closure, and log files assigned to a daemon's output before it
starts, remain reportable. Nine fire-and-forget launchers remain policy
reports. Opt-in global-state, API-shape, and detached-goroutine findings were
sampled without expanding default policy.

## Batch 38

Ten repositories and seventeen modules were selected. Thirteen modules loaded
and scanned fully; Neoray and the node monitoring agent produced useful
partial analysis around dependencies that exclude the host platform, and two
directories hold no buildable package.
Final scans produced 579 unique diagnostics after one bounded
correction:

- lockorder's loop-variant rule follows a call whose argument is selected by
  the iteration, such as the per-session state a cleanup loop fetches before
  locking it, so locking a different state each time is not recursion; a
  lookup with a fixed key still returns the same mutex and stays reportable.

The correction removed one CodeKanban session-cleanup finding. Precision
round 40 labels it and five nearby true positives: a file leaked on a copy
error inside an archive walk, a response leaked on a status check, a
testdata directory never closed, an error check that tests the wrong
variable, and a response leaked on provider error statuses. Four
fire-and-forget launchers remain policy reports. Opt-in global-state,
API-shape, and detached-goroutine findings were sampled without expanding
default policy.

## Batch 39

Ten repositories and eleven modules were selected. Nine modules loaded and
scanned fully; autobump could not build here and one terraform tools
directory holds no buildable package.
Final scans produced 882 unique diagnostics after two bounded
corrections:

- processownership treats a command stored into caller-owned storage on
  every path to Start, such as a receiver field a later goroutine waits
  through, as transferred to that owner; a command held only by a local
  struct stays reportable; and
- a value ranged from a map or slice that the caller owns is itself
  caller-owned, so a goroutine that closes a channel taken from a receiver's
  map transfers its obligation across the call boundary.

The corrections removed one Istio Envoy driver finding and two policyserv
pubsub findings. Precision round 41 labels two of them and six nearby true
positives: two responses leaked on status checks or decoded without a close,
a row set leaked on a scan error, a file leaked on a write error, a ticker
never stopped, and a test response never closed. A test-deadline timer
consumed by a launched goroutine remains reportable until resourcelifetime
classifies launched consumers. Six daemon and worker launchers remain policy
reports. Opt-in global-state, API-shape, and detached-goroutine findings were
sampled without expanding default policy.

## Batch 40

Ten repositories and sixteen modules were selected. Eleven modules loaded and
scanned; ksail's root and desktop modules exhaust memory under their
Kubernetes and Wails dependency trees, two go-sdk examples need go.mod
updates, and a tree-sitter cgo harness and kandev's sqlite tests do not
build here. Final scans produced 2,310 unique diagnostics after three
bounded corrections:

- the resourcelifetime flow now carries its unknown mark across block
  boundaries, so an opaque consumption keeps every later return unknown
  instead of only returns in the same block;
- lockorder reads Go's comma-ok convention when judging acquire-for-caller
  contracts: a return whose trailing Boolean result is the constant false is
  not a successful return, so a claim helper that unlocks before returning
  false and holds on returning true is acquiring for its caller; and
- the resourcelifetime classifier treats a struct literal holding a value
  derived from the resource, such as a type-asserted response body, as
  carrying it when the literal is handed to a function value.

The corrections removed a spirit checksum finding, a kandev lock finding,
and a kandev SPDY upgrade finding. The first correction also retires a
routatic catalog finding that was a genuine leak, a row set overwritten by
a second query before its deferred close: the deferred closure closes
whichever value the cell holds at exit, which the model records as unknown
rather than proven. Precision round 42 labels the three false positives and
four nearby true positives: a transaction leaked when its connection-id
query fails, a row set leaked on an iteration error, an email file never
closed, and a cancel deferred inside a ticker loop. Four fire-and-forget
editor and browser launches appear only under the opt-in detached-process
audit. Opt-in global-state, API-shape, and detached-goroutine findings were
sampled without expanding default policy.

## Batch 41

Nine repositories and twenty-three modules were selected and scanned
through , the first batch on that path. Twenty modules loaded and
scanned; nuon's root module and maglev need module downloads the scan
forbids, and one panurus tools directory holds no buildable package. Final
scans produced 993 unique diagnostics after one bounded correction:

- a value selected from an element of a slice, array, map, or range
  iteration is shared with whatever else holds that aggregate, so a started
  command loaded from a local slice and a worker whose completion signal is
  reached through such an element are unknown rather than violated.
  processownership skips the command before asking the flow, and
  goroutineownership records a shared-storage-signal outcome after the exact
  join proofs fail.

The correction removed a cocoon benchmark launcher finding under the opt-in
detached audit and two MikroDash session launches. Precision round 43 labels
the three and five nearby true positives: a broadcast goroutine that sends
on an unbuffered result channel after the caller returned on error, two
attachment files never closed when a write fails, a calendar poll ticker
leaked when a follow-up lookup fails, and a prepared statement leaked when a
later prepare fails. A vault readiness probe that returns without closing a
healthy response is a true positive in a module too deep for the replay's
module budget and is not labelled. Opt-in global-state, API-shape, and
detached-goroutine findings were sampled without expanding default policy.

## Batch 42

Ten repositories and thirty-nine modules were selected after a generated
client library of several hundred modules was swapped out; the scan driver
now keeps at most the shallowest two dozen modules of a repository. Twenty
modules loaded and scanned; hoop's services and several substrate tool
directories need module downloads or hold no packages, and the GPU exporter
requires a newer toolchain. Final scans produced 1,716 unique diagnostics
with no bounded correction: every default ownership finding reviewed was a
defect or a policy report. Precision round 44 pins eight of them: a struct
returned in the same statement as the Unmarshal that fills it, a file
closed by a defer inside a copy loop, accept-loop workers left unjoined when
Accept fails for another reason than shutdown, a virtiofsd log file leaked
and a virtiofsd command killed without being reaped when socket readiness
times out, a containerd stdout file and process leaked when the client
fails to connect, and a temporary file created only for its name. Nine
logged-and-returned errors and four fire-and-forget browser launches are
policy and opt-in reports respectively.

## Batch 43

Twenty-five repositories and thirty-eight modules were selected, the first
batch at the wider size. Twenty-seven modules loaded and scanned; the rest
need module downloads the scan forbids or hold no packages. Final scans
produced 3,422 unique diagnostics, two thirds of them opt-in global-state
reports from operator codebases, with no bounded correction: all
twenty-four default ownership findings reviewed were defects or policy
reports. Precision round 45 pins eight of them across five repositories,
including diecast's Serve, which launches several server goroutines that
each send on one unbuffered channel it receives from once, and kubectl-gs's
device-token wait, whose timeout goroutine sends after the select has
returned. Five fire-and-forget browser launches appear only under the
opt-in detached-process audit.

## Batch 44

Twenty-five repositories and sixty-four modules were selected. Fifty-two
modules loaded and scanned; the rest need module downloads the scan forbids
or hold no packages. Final scans produced 1,008 unique diagnostics after one
bounded correction:

- the detached-process audit now decides "handed on" by whether the handle,
  a projection of it, a pipe it returned, or an aggregate holding one of
  those reaches a call, store, return, or send; derivation stops at a scalar,
  so logging or returning the child's PID no longer turns a fire-and-forget
  launch into a default missing-wait report.

The correction moved an agent-filesystem daemon re-exec, which prints the
child's PID and returns, from the default check to the opt-in detached
audit; it is not labelled because the audit still reports the same position
under the same analyzer. Precision round 46 pins seven true positives across
five repositories. A PID-only benchmark helper, a TTY handle leaked when the
device is not a terminal, and a heartbeat ticker never stopped are true
positives in modules beyond the replay's module budget and are not
labelled. Four fire-and-forget browser launches appear only under the opt-in
detached-process audit.

## Batch 45

Twenty-five repositories and fifty-one modules were selected. Thirty-five
modules loaded and scanned; the rest need module downloads the scan forbids,
hold no packages, or fail to build. Final scans produced 980 unique
diagnostics after two bounded corrections:

- resourcelifetime treats the closed branch of a receive from a ticker's or
  timer's own channel as infeasible, since time documents that Stop never
  closes the channel, so a loop ranging that channel forever has no
  reachable exit to leak on; and
- lockorder resolves a deferred function's result cells to the value stored
  on the returning path, so a nil error behind a defer counts as a
  successful return and the acquire-for-caller contract is recognized.

The corrections removed a coroot scrape loop finding and a libocr
transaction-constructor finding. Precision round 47 labels them and eleven
nearby true positives: a probe that opens a device without closing it, a
server goroutine sending on an unbuffered channel received from once, a lexer
goroutine left blocked by an early error return, three responses and a gzip
reader leaked on error paths, a generated file and a key file never closed,
a temp file created only for its name, a deferred close inside a loop, and
an SSH command left running when its archive peer fails to start. One infra
finding is a known gap rather than a labelled false positive: a transaction
whose Rollback method releases only when a completion flag is unset is
summarized as releasing nothing, so a deferred rollback helper does not
settle it; a conditional-release summary would need a new fact mask.
Fifteen logged-and-returned errors are policy reports and one fire-and-forget
agent launch appears only under the experimental detached audit.

## Batch 46

The final six repositories bring the audit to five hundred. Eight modules
were selected and five loaded and scanned; rtlamr, signatory's root, and the
CloudStack provider's root need module downloads the scan forbids. Final
scans produced 232 unique diagnostics with no bounded correction: the three
default ownership findings reviewed are defects, and precision round 48 pins
all of them: a key file created only for its path, a button-press response
never closed, and a pipe reader goroutine left blocked when closing the
writer fails. One logged-and-returned error is a policy report.

## Batch 47

Forty-nine repositories already cloned but outside the five-hundred selection,
scanned at a777a05. Eight failed to build. Enabling every check produced 718
diagnostics and the default tier produced 226, of which 80 sit in test files;
the 146 production findings across twenty-six repositories are what a user
sees. The gap matters: sixty-three per cent of the wider number is
goroutineownership/detached, an experimental check, and reading it as the core
unjoined check would have made the corpus look far noisier than it is.

Twenty-two findings were reviewed in detail across every check that fired.
One bounded correction: lockorder/recursive-acquire reported a Lock and Unlock
paired inside a loop body in sozercan/vekil, whose mutex is wrapped in a type
whose Unlock returns early on a nil receiver. The release is real and, for a
receiver taken from a field, the guarded path is unreachable, but it is not
provable on every path, so the lock stayed proven held into the next
iteration. Recording the weaker answer separately keeps a return that still
holds the lock reportable while declining the recursion claim, which is the
one that needs the lock to be proven held.

The rest were defects. exitpolicy reports only exits that follow a registered
defer, and skips the ones that precede any, which is what leaves a temporary
directory and a Postgres container behind on the paths it names. deferinloop
named a pagination loop accumulating response bodies. concurrentcapture named
a batch handler whose per-request goroutines take index and body as parameters
but capture and write two enclosing strings, and a goroutine that assigns a
captured error with = where the line above it uses :=. Resource findings named
bodies left open on non-200 paths, a file never closed at all, and a ticker
never stopped.

## Batch 48 (triaged)

Twenty-five fresh repositories with root Go 1.26/1.27 directives were pinned
and scanned with gohawk `cec1074c40560eabe31f5c91488faa8533299dac`, using
Go 1.27.0 on linux/amd64. Selection excluded recorded precision cohorts and
leadgen study repositories, then took eligible candidates from a GitHub search
for recently updated Go repositories with 500–5,000 stars and size below 30,000.
The scan included all checks and test source, capped at three modules per
repository with 180-second scan deadlines. CGO was disabled. No candidate test
binaries, generation commands, or repository scripts were executed.

All 357 original diagnostic locations now have a triage disposition:

| Disposition | Locations |
| --- | ---: |
| True positive: defect or concrete hazard | 62 |
| True positive: policy-only audit | 23 |
| False positive | 79 |
| Inconclusive: driver/timing-dependent SQL tests | 5 |
| Retired goroutine detached check | 188 |

The TSV retains `true-positive` for both kinds of accurate report; policy-only
reasons begin with `Policy-only`. These are not 85 confirmed runtime bugs.
Five SQL findings were reclassified from false positive after checking the
cancellation timer and connection-state assumptions. They are not regression
labels requiring either presence or absence; see the follow-up assessment.
`retired-check` preserves historical reports without treating silence from a
deleted check as a precision improvement. This is source review, not runtime reproduction,
and the original scan is not a census of the current binary.

Twenty-four scans completed without recorded errors. `ekristen/aws-nuke`
timed out and remains **incomplete**, not clean; triaging the available findings
does not complete that scan. Zero reported findings do not establish recall.

Artifacts:

- [Repository pins and scan status](batch-48.tsv).
- [Per-repository scan counts, errors, and verdict totals](batch-48-scans.json).
- [Every finding, with reviewed verdicts and rationale](batch-48-findings.tsv).

The initial 25 reviewed findings motivated four corrections in `a728a75`:
iteration-local captures, stable computed lock guards, completion-channel
accessor handoffs, and registered cleanup capturing a reassigned resource.
Precision round 49 keeps those 16 true positives and nine corrected false
positives. Its replay passed at that revision, with all repositories scannable.
The WaitGroup finding in certificate-transparency-go was corrected to a true
positive: Done runs before deferred response cleanup, so Wait can return early.

The initial completed triage added 75 false-positive labels to the audit
ledger, not to the passing replay cohort. These began as a follow-up backlog;
the timer/getter corrections and their validation are recorded below:

- 61 channel timer/ticker reports, mostly in Okteto. The reviewed Go 1.26
  modules use Go 1.23+ timer semantics: unreachable channel timers can be
  garbage-collected without Stop. No `asynctimerchan` override was found in
  the reviewed Okteto/Flamingo sources. This does not apply to AfterFunc
  callbacks or workers that retain a timer indefinitely.
- Nine reports in MariaDB operator's bundled MySQL driver test module:
  canceled/failed acquisitions, and prepared statements released through
  the parent test database's Close. These are bundled dependency tests,
  not operator production defects.
- Five codex2api proxy locations where repeated `account.Mu()` calls return
  the same mutex but the analyzer treats the call results as separate locks.
  Every local RLock/RUnlock pair is completed before another loop iteration.
  One location carries both recursion and contradictory-order check IDs.

True positives include unclosed temporary/generated files, HTTP error paths
before Body.Close, row Scan errors before Close, a transaction error path
without rollback, and early completion notification. Libopenapi's callback
count getter also writes its cached count and reference map under RLock;
this is an experimental concurrency hazard, not a reproduced race. The
policy-only findings are process-exit/skipped-defer and desktop-opener audits.

The previously inconclusive codex2api `auth/store.go:8641:19` report is a
true-positive lock-order hazard. Candidate-scoped tracing identifies the
reverse edge, and shared-account callers hold Account.mu through
recomputeEffectiveAutoPause → resolveEffectiveThreshold →
GetGlobalAutoPause5h/7dThreshold, which acquires Store.mu. This contradicts
EnabledGrokAccounts → IsGrokAPI's Store.mu → Account.mu order. The first
recorded reverse edge involves a fresh account, but the shared-account
callsites rule out a fresh-object-only explanation. No runtime deadlock was
executed. See the [follow-up assessment](batch-48-followup.md).

The broad `goroutineownership/detached` check was retired in `2acff9c`.
Its 188 original locations retain source-review notes, including intentional
background work, missed receiver-owned joins, and actual endless workers.
Those notes do not create replacement diagnostics. Seven detached-only
labels from older executable cohorts were removed, while focused unjoined
and abandoned-send checks remain. A scoped round-49 goroutine replay after
retirement retained all four FP suppressions and its one TP.

Validation for this record is ledger consistency and pinned-source/location
checks. No analyzer implementation or precision baseline is changed here.
Follow-up `049d8c9` removes the unconditional channel-timer obligation and
resolves visible mutex getters while declining opaque ones. Round 50 preserves
the sampled corrections and nearby controls. Round 51 fixes two further SQL
false positives. Of the seven remaining SQL findings, two are callback-parent
cleanup false positives and five are now inconclusive after reassessing timing
and driver assumptions. The follow-up records the callback evidence gap.

The subsequent bounded caller-context implementation resolves those two
callback-parent false positives and adds them to round 51, now containing four
false-positive controls and one true-positive control. The five inconclusive
findings are unchanged. See the bounded cleanup follow-up in
[batch-48-followup.md](batch-48-followup.md).

## Batch 49

One hundred new repositories were selected at pinned revisions, targeting root
Go 1.26/1.27 modules and excluding previously audited and outreach repositories.
The [selection ledger](batch-49.tsv), [scan metadata](batch-49-scans.json), and
[per-finding review](batch-49-findings.tsv) preserve all selected repositories,
including those with no findings or incomplete scans.

The pinned scanner was `10e8a5c`, before the approved `exitpolicy` retirement in
`4d3212f`. Its 364 reported locations are fully triaged:

- 193 true positives, including explicitly identified policy-only findings;
- 29 false positives;
- 5 inconclusive findings;
- 137 historical `exitpolicy` findings, recorded as retired rather than active.

These are static source-review verdicts, not runtime reproductions. In
particular, a process intentionally launched without a waiter is not automatically
a demonstrated production failure. No candidate tests, generators, or application
programs were executed. The scan used all checks and test source, CGO disabled,
at most three modules per repository, four concurrent scans, and a 180-second
limit per module. The metadata pins the binary and runner hashes.

Ninety-two repositories completed the selected-module scan. Eight were incomplete:
`casosorg/casos`, `kai-scheduler/KAI-Scheduler`, and `ymtdzzz/otel-tui` hit time
limits; `thought-machine/please` and `vbauerster/mpb` had nested modules with
missing sums; `BeryJu/gravity` had a CGO-dependent utility; `golang/crypto` had
assembly-generator modules using incompatible older tooling; and
`kubernetes-sigs/sig-storage-lib-external-provisioner` had a failed example-module
load. Exact captured errors are retained. Findings from successfully loaded
packages were reviewed, but incomplete repositories are not clean scans.

### Check-level reassessment

- **Stable guards and path correlation:** six go-drive lock findings and one
  Cerbos exporter finding miss repeated stable predicates. Retain the checks;
  prefer a bounded extension of existing feasible-path evidence that distinguishes
  unrelated field writes from changes to the guarded value. Do not equate repeated
  arbitrary calls or mutable field reads. Moderate complexity; decline unknown
  paths rather than adding project-specific exemptions.
- **Returned or retained ownership:** Cerbos variadic closer collections,
  Terway retained callbacks/logger options, and kube-vip's returned Unlocker need
  aggregate or escape evidence. Retain the focused checks. Reuse shared storage
  and call effects where proof is local; opaque retained callbacks should become
  unknown, not framework-name exceptions. Collection-wide cleanup is a separate,
  higher-complexity proof and must account for early exits and aliases.
- **SQL rows:** yarr exposes complete single-result-set iteration and cleanup
  through a caller-owned transaction. Retain the check and first investigate
  reusing existing iterator and bounded caller-cleanup evidence. Neither arbitrary
  Next methods nor every parent Close can be treated as cleanup. Moderate scope.
- **Compression:** five gzip-reader reports concern wrappers that own no external
  resource; three writer reports concern deliberately aborted/discarded output.
  Reassess the obligation itself before expanding dataflow. Reader Close does not
  close the input or validate checksums. A useful writer check concerns publishing
  unfinished output, not every missing Close on an abandoned stream. Changing
  default policy remains a separate decision, not a silent suppression here.
- **Worker completion:** dskit counts completed loop work, buildtools joins a
  strided collection, Flamingo joins through a request registry, and mpb returns
  a WaitGroup-owning receiver. Retain focused checks, investigate existing
  completion/returned-owner evidence first, and decline opaque registry ownership.
  Arbitrary collection synchronization is substantially harder than a local join;
  do not add an unbounded framework model to accommodate it.
- **Failed acquisition:** go-drive's already-canceled OAuth request cannot acquire
  a successful response through its visible transport path. Investigate extending
  existing canceled-acquisition evidence only where the transport contract is
  known; arbitrary RoundTrippers need not honor cancellation.

The five inconclusive reports retain their individual uncertainties: timing or
harness assumptions in three tests, a producer whose completion depends on input
cardinality, and conditional retention in Please's returned error stack. No claim
of universal cleanup is made for those cases.

This commit records the audit, not analyzer fixes. Unfixed false positives are
not added as passing regression labels. Follow-up fixes are authorized, but each
still needs a minimized fixture, nearby positive controls, and replay evidence.
Validation checked unique finding keys, complete review coverage, all 100 revision
pins, and every reported source location against the retained pinned checkouts.

The [batch-49 follow-up](batch-49-followup.md) records 25 corrected false
positives, their bounded uncertainty tradeoffs, and four still-unresolved cases.
Round 52 preserves 24 corrected cases and six neighboring true positives;
mpb's correction has a separate root-package replay and local fixture because
its example modules cannot load in the whole-repository runner. In all cases,
the original batch verdicts remain the historical baseline.

## Batch 50

Two hundred and fifty new repositories are pinned in the
[selection ledger](batch-50.tsv). [Selection provenance](batch-50-selection.json)
records the GitHub search pages, selected repository metadata, root Go
directives, and historical exclusions. All selected repositories require Go
1.26 or Go 1.27.0 and can be considered by the fixed local Go 1.27.0 toolchain;
this does not imply their packages will load successfully. The selected slice
has 500–1,197 stars and repository size below 30,000, within the established
500–5,000-star search bounds. Prior audited, in-flight, and outreach repositories
were excluded before selecting 250 of 258 eligible candidates.

The scanner is pinned to `6ed616c`, with binary and runner hashes in the
[scan metadata](batch-50-scans.json). Four repository scans run concurrently,
with at most three modules per repository and 180 seconds per analysis command.
The profile enables all checks and includes test source, but does not execute
candidate tests, generators, or applications. CGO is disabled, toolchain
auto-download is disabled, and module files are read-only.

All 250 repositories completed their bounded scan attempt across 319 module
entries: 195 scans completed and 55 were incomplete. All 444 reported findings
were individually reviewed: 363 true positives, 55 false positives, and 26
inconclusive. The [per-finding reviews](batch-50-findings.tsv) preserve exact
positions, check IDs, revisions, and source evidence. Incomplete scans are not
counted as clean; 102 completed scans reported no findings within the selected
scope. Original scan verdicts remain separate from later corrected replays.

The [batch assessment](batch-50.md) records the principal evidence gaps,
operational limitations, and correction status. Not every confirmed false
positive was fixed: unresolved ownership, protocol, and path-correlation gaps
remain explicit rather than receiving repository-specific exceptions.

The [bounded follow-up](batch-50-followup.md) verifies eight additional FP
removals using existing machinery. Nineteen original FP locations now have
correction evidence; 36 remain unresolved. Round 53 passes 14 FP labels,
including six earlier Nylon corrections, and 11 true-positive controls.

The [complete remaining-site review](batch-50-remaining.md) reviews all 36
together and adds 11 bounded corrections, for 30 corrected original locations
and 25 unresolved. Its per-site ledger separates review from verified fixes.
Round 54 passes seven FP and ten TP labels; Neffos has a separate root-package
replay because unrelated example modules cannot load in the full harness.

## Batch 51

Another 250 fresh repositories were selected at full revision pins, excluding
prior audits, regression cohorts, outreach and in-flight selections. The
[selection ledger](batch-51.tsv) and [selection provenance](batch-51-selection.json)
record the Go 1.26/1.27.0, 500–5,000-star, nonfork/nonarchived and size-bounded
profile. Selection screened 2,133 fresh candidates and chose 250 of 259 eligible
repositories; the selected slice has 800–5,000 stars.

The [scan metadata](batch-51-scans.json) fixes analyzer source `f105aa55`, binary
and runner hashes, pins, module scope and captured errors. Three repository
workers attempted 320 module entries with all checks and test source enabled,
CGO and toolchain auto-download disabled, read-only modules and bounded timeouts.
No candidate tests, generators or applications were executed.

All 250 scan attempts and all 421 original findings are individually reviewed:
325 true positives, 85 false positives and 11 inconclusive. Of the repositories,
193 completed the selected-module scan and 57 were incomplete; 106 completed
scans reported no findings. Partial scans are not counted as clean. The
[per-finding ledger](batch-51-findings.tsv) preserves every original verdict,
exact source position, check ID and source-review explanation.

The [assessment](batch-51.md) records eight bounded evidence improvements,
their accepted coverage gaps, and the remaining check-level questions.
[Correction receipts](batch-51-corrections.tsv) identify 24 verified original
false-positive removals; 61 original false positives remain unresolved. Four
focused analyzer commits are separate from this audit record. Round 55 passes
21 labels across eight fully scannable repositories: ten false positives remain
absent and eleven true positives remain present. Four incomplete repositories
have explicitly scoped pinned replays instead of misleading whole-repository
passes.

Validation includes `make verify`, a full race run followed by race reruns of
subsequently changed packages, rounds 53/54 control replays, round 55, scanner
unit tests, and ledger checks against the pinned checkout HEADs and source
locations. The cohort measures reviewed findings, not recall or universal
repository safety.

## Batch 52

The first additional 250-repository batch uses frozen source `c9609c6`.
[Selection](batch-52.tsv), [provenance](batch-52-selection.json) and
[scan metadata](batch-52-scans.json) preserve exact pins and static-only scope.
After exhausting fresh Go 1.26+ candidates, this batch includes compatible
Go 1.25–1.26.4 projects. There were 317 module entries, 200 complete repositories,
50 incomplete repositories and 111 quiet complete scans.

All 455 findings are [reviewed](batch-52-findings.tsv): 341 true positives,
107 false positives and seven inconclusive. Two false positives have
[verified corrections](batch-52-corrections.tsv); 105 remain unresolved.
The [assessment](batch-52.md) records bounded process and range-capture fixes,
remaining evidence families, and a baseline-reproduced Go type-loader race
limiting the optional architecture race gate. Canonical `make verify` and
changed-package race tests pass; round 56 passes two FP and six TP labels.

## Batch 53

The second additional 250-repository batch uses the frozen `c9609c6` baseline.
The [manifest](batch-53.tsv), [selection provenance](batch-53-selection.json)
and [scan metadata](batch-53-scans.json) record the pins and bounded static-only
profile. The selected repositories declare Go 1.25; older compatible projects
were included after the fresh Go 1.26+ selection pool was exhausted.

All 386 findings are [individually reviewed](batch-53-findings.tsv): 322 true
positives, 48 false positives and 16 inconclusive. Of 250 repositories and 307
module entries, 201 repositories completed and 49 were incomplete; 119 complete
scans were quiet. Partial scans are not clean, and silence does not measure recall.

The [assessment](batch-53.md) groups captured cleanup cells, no-body HTTP
responses, infeasible paths, retained owners, indirect completion, process
identity and producer cardinality. All 48 false positives remain unresolved;
no correction or check retirement is claimed. The ledger is verified against
every pinned checkout and original finding.

## Batch 54

The third additional 250-repository batch uses the same frozen `c9609c6`
baseline. [Selection](batch-54.tsv), [provenance](batch-54-selection.json)
and [scan metadata](batch-54-scans.json) preserve the static-only profile and
full pins. There were 312 module entries: 203 repositories completed, 47 were
incomplete, and 125 complete scans were quiet.

All 311 findings are [reviewed](batch-54-findings.tsv): 242 true positives,
65 false positives and four inconclusive. One false positive has a
[verified scoped correction](batch-54-corrections.tsv) from the earlier
process-identity fix; 64 remain unresolved. The [assessment](batch-54.md)
records retained-owner, feasible-path, callback, protocol and lock-evidence
gaps without claiming those unresolved families are fixed.

## Batch 55

The fourth additional 250-repository batch completes the requested 1,250
repositories including batch 51. [Selection](batch-55.tsv),
[provenance](batch-55-selection.json) and [scan metadata](batch-55-scans.json)
preserve the frozen original baseline and static-only scope. Of 292 module
entries, 194 repositories completed and 56 were incomplete; 123 complete
scans were quiet.

All 592 findings are [reviewed](batch-55-findings.tsv): 520 true positives,
69 false positives and three inconclusive. Ten have
[verified corrections](batch-55-corrections.tsv) through the shared lifecycle
summary fix; 59 false positives remain unresolved. Round 57 passes ten FP
and two TP labels. The [assessment](batch-55.md) explains exact callback
completion, remaining evidence gaps, generated-code clustering, and a
source-backed queued-writer lock hazard—not a reproduced production incident.

## Historical 500-repository audit summary

Five hundred repositories were reviewed across forty-six batches. The
recorded rounds pin every corrected false positive and a sample of true
positives, and `make precision-regression` replays them all. The last five
batches, one hundred and one repositories at the wider batch size, needed
three bounded corrections between them, each a structural predicate at an
existing decision point with a fixture and a pinned link, and no correction
in three of the five. The remaining known gap is a conditional-release
method summarized as releasing nothing, recorded under batch 45.
