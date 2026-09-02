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
