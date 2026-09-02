# 500-repository precision audit

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
false positives and fourteen nearby true positives: an nginx config file
leaked when its template fails, a control file never closed, three response
bodies never closed or leaked on status and length checks, a temporary file
leaked when its removal fails, an stderr scanner goroutine leaked when pipe
creation fails, a discarded response, three more responses leaked on status
checks or discarded, a retry timer never stopped, a temporary file never
closed, and a defer inside a writer loop.

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
Precision round 37 labels five of those false positives and five nearby true
positives: an SBOM output file never closed, a gzip reader never closed, a
file leaked when its Stat fails, a discarded signal-context cancel, and a
response leaked on a method check before its deferred close.

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
