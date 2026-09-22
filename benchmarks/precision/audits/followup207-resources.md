# Resource follow-up: active 207-finding queue

This continues the 140 resource false positives left by
`resource-tightening-2026-09-22.tsv`. All 140 have now been replayed, but their
resolution remains in progress.
`followup207-resources.tsv` preserves the original verdict separately from
replay status and includes the 72 original resource true-positive controls.

## Checkpoint: guarded captured cleanup v3

- Twenty additional false positives are absent after a successful baseline and
  corrected replay. They cover a zero-client HEAD request (one), returned
  standard loggers (two), a returned body narrowed to `io.Reader` (one), and
  manager-retained resources exposed through returned handles (four), and
  exact local header-only HTTP servers (two), private logger installers (two),
  shared deferred process-termination evidence (one), visible error predicates
  through direct calls and immutable captures (four), and a merged fallback
  acquisition handled by shared feasible-path and uncertain-cleanup evidence
  (one), an exact standard buffer constructor behind a compressor (one), and a
  called closure that nil-guards an exact captured response's `Body` before
  closing it (one).
- All 66 baseline-detected true positives remain detected in the current
  all-check replay. Five wg-portal
  misses and the previously documented piko WebSocket miss remain baseline
  misses; none is newly lost here.
- 119 false positives still report and remain active. None is left without a
  current package-scope replay.
- Geesefs `core/cfg/logger.go:37:16` is absent in both binaries in this replay,
  unlike earlier canonical replays. This profile-dependent baseline absence
  remains unresolved and is not counted as a correction.

The first returned-logger candidate hid quickfix's genuine first-file leak:
historical aggregate containment saw a logger constructed only after an early
error return. That candidate was rejected. The retained implementation requires
logger construction to dominate the actual return; a minimized regression
keeps the earlier leak diagnostic while accepting the second returned file.

The branch-local retention change accepts a retaining helper in one branch
when the caller's own cleanup exists only in a mutually exclusive branch.
Reachable later cleanup still establishes caller ownership, preserving the
PNG-encoder error-path regression. The private-helper fallback now fixes
shijuvar's two reports by reusing strict retention evidence at actual global
stores before every normal return. Constructor calls alone, local spills,
callback captures, and caller-owned cells do not establish that handoff.
The initial fallback counted a captured cell's spill and hid a conditional
testing-cleanup regression; that candidate was rejected and the witness
boundary tightened. The existing negative fixture is retained.

The error-predicate extension proves a narrow implication: the exact nil
acquisition error makes every reachable normal return literal false. It does
not infer application-specific failure semantics. Captured callbacks require
the same visible function through every lexical construction, immutable cells
at each layer, and one 1000-step budget shared with storage/effect queries and
the nil-error flow. Recursive predicates, mutable or escaped cells, rewritten
errors and deferred return mutation remain unknown. The existing acquisition
error family was moved together into `acquisition_errors.go`; reporting still
uses the original resource flow and authoritative guard classification.

The byJoey fallback-open site is a combined shared-flow correction, not another
error-predicate case. Its failed-acquisition return is inactive; its successful
return carries `ambiguous-cleanup-value` from the deferred merged file's Close.
The final decision is `opaque-consumption`, not a claim of proven release.
The focused trace is `.build/followup207-error-guard-byjoey.trace.jsonl`.

Standard `bytes.NewBuffer` and `bytes.NewBufferString` constructors now receive
the same existing memory-writer boundary as a local buffer allocation. The
Pulsar compressor site uses the former. This does not prove arbitrary writes
infallible: custom factories and mixed external writers still report, and
`require-memory-writer-close=true` retains both constructor obligations.

## Evidence boundaries

A called literal that loads an exact captured `http.Response` cell, checks
its `Body` against nil, and closes a second load of that `Body` is classified
as unknown consumption, not as proven release: the two loads are not proved
equal. The boundary requires the cell to still hold the acquisition at the
call, no visible pointer escape of the response or its cell (other closures,
arguments, stores, map or channel sends), a body-nil guard alone, and `Close`
on every return under that guard. Extra Boolean conditions, cleanup of a
different response, and direct or helper replacement of `Body` keep the
diagnostic. Replacing the whole captured cell was already an accepted coverage
gap before this boundary and remains one. This corrects the Kruise e2e
framework site; the full 83-scope replay retained all 66 controls with zero
new diagnostics. Focused resource, architecture, lint and race gates passed.

The HEAD boundary requires an unchanged zero-value local client and a direct,
unchanged `NewRequest("HEAD", ...)` result. Explicit client transports,
timeouts, helper escapes and request mutation do not qualify. The mutable
global default transport makes this acquisition uncertainty, not proof that
closing is unnecessary.

The local-server boundary requires an exact unchanged `httptest.NewServer`
endpoint and fully visible header-only writer effects. Dynamic/framing headers,
body writes, flushing, hijacking, writer escapes, redirects, and visible default
client/transport overrides reject it. Hidden cross-package mutations of global
HTTP defaults remain a stated coverage gap. It corrects the two auth Reset
sites; the eight Set sites still have unresolved dynamic-header effects.

Returning a `log.Logger` retains the writer exposed by `Logger.Writer`.
Discarding the logger or returning before its construction does not qualify.
Interface narrowing preserves the dynamic cleanup-bearing body only when the
shared storage model proves the original response projection is unchanged.

Fresh owned-field summaries now decline a new caller obligation when the
constructor also puts the same acquired resource into an already external map
or owner. This does not prove manager cleanup. Local scratch maps still allow
ordinary fresh ownership, and direct file obligations remain independently
checked inside the constructor.

## Reproduction

All candidate repositories were used only for static analysis. The 83 pinned
package scopes were replayed with the direct CLI profile:

```text
gohawk -enable-all -gohawk-include-tests -json PACKAGE
PROTO_REPORTER=text CGO_ENABLED=0 GOWORK=off GOMAXPROCS=2
GOFLAGS=-mod=readonly -p=2 GOTOOLCHAIN=local
```

Repository revisions, package/module scopes, command, environment, exit status,
stdout and stderr receipts are recorded under `.build/followup207-resource-baseline`
and `.build/followup207-captured-body-v3`, with paths in the TSV. Exit zero with no
findings and exit three with valid diagnostic JSON are successful analyses;
failed loads are not treated as absence.

- Baseline: `.build/gohawk-followup78-goroutines-v5`, SHA-256
  `5516cad4c83bffd8dca28713df53f8d3d1a463b838c23d302da9e10ddc257419`.
- Candidate: `.build/gohawk-followup207-captured-body-v3`, SHA-256
  `cecd0be4d83937d9ee6e8a6b16260cc69976a12e951cabea89f88a92cdde67a9`.
  The earlier memory-writer checkpoint binary was
  `.build/gohawk-followup207-memory-writer-v1`, SHA-256
  `193c77d2c11219ba9c77f210e1b87f0a003d881e51b52a77c345778b5441c696`.

All 83 package scopes completed successfully. Comparing every resource
diagnostic in those scopes, not just the labelled sites, found zero new
diagnostics and twenty removed diagnostics. The additional package scopes
have matching successful immutable-baseline receipts; absence
in a newly scanned scope alone was not counted as a correction.

The earlier HTTP-v1 all-check `megaease/easeprobe ./daemon` replay timed out at 240 seconds,
with sampled peak RSS around 18 GB. Its receipt is retained as a failure and
none of its sites is counted absent. This package has no HTTP acquisitions;
the candidate also contains contemporaneous shared-flow edits being investigated
separately. The preceding v5 candidate completed the same package. Failed-process
children were explicitly terminated; subsequent replay tooling uses a process
group timeout.

An isolated `-enable=resourcelifetime -gohawk-include-tests -json ./daemon`
rerun using the same HTTP candidate completed normally and retained the
`daemon_test.go:103:14` true positive plus the four false-positive sites. This
supports isolating the performance regression elsewhere, but is not substituted
for the required all-check receipt. The corrected combined retention-v2
candidate subsequently completed that scope in 6.96 seconds, with the true
positive retained. Its canonical receipt is now used in the ledger.

Focused resource, lifecyclefacts and architecture tests pass. Focused
lifecyclefacts and resource race tests pass (the latest resource race run
completed in 123 seconds). Targeted lint reports zero issues. Repository-wide final validation
and the rest of the active findings are still in progress.
