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
