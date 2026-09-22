# Resource follow-up: active 207-finding queue

This continues the 140 resource false positives left by
`resource-tightening-2026-09-22.tsv`. It is not a completed audit of all 140.
`followup207-resources.tsv` preserves the original verdict separately from
replay status and includes the 72 original resource true-positive controls.

## Checkpoint: HTTP v1 (one all-check replay incomplete)

- Ten additional false positives are absent after a successful baseline and
  corrected replay. They cover a zero-client HEAD request (one), returned
  standard loggers (two), a returned body narrowed to `io.Reader` (one), and
  manager-retained resources exposed through returned handles (four), and
  exact local header-only HTTP servers (two).
- 65 baseline-detected true positives remain detected in the current all-check
  replay. The remaining control belongs to a timed-out package, not a lost
diagnostic. All 66 were retained in the preceding v5 checkpoint. Five wg-portal
  misses and the previously documented piko WebSocket miss remain baseline
  misses; none is newly lost here.
- 82 false positives still report. Another 43 are outside these replayed
  package scopes and four belong to the timed-out package; all remain active.
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
PNG-encoder error-path regression. This alone does **not** fix shijuvar's two
reports: its helper-to-global-logger retention summary is still missing.

## Evidence boundaries

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

All candidate repositories were used only for static analysis. The 56 pinned
package scopes were replayed with the direct CLI profile:

```text
gohawk -enable-all -gohawk-include-tests -json PACKAGE
PROTO_REPORTER=text CGO_ENABLED=0 GOWORK=off GOMAXPROCS=2
GOFLAGS=-mod=readonly -p=2 GOTOOLCHAIN=local
```

Repository revisions, package/module scopes, command, environment, exit status,
stdout and stderr receipts are recorded under `.build/followup207-resource-baseline`
and `.build/followup207-resource-http-v1`, with paths in the TSV. Exit zero with no
findings and exit three with valid diagnostic JSON are successful analyses;
failed loads are not treated as absence.

- Baseline: `.build/gohawk-followup78-goroutines-v5`, SHA-256
  `5516cad4c83bffd8dca28713df53f8d3d1a463b838c23d302da9e10ddc257419`.
- Candidate: `.build/gohawk-followup207-resource-http-v1`, SHA-256
  `161c845834d8495cdb6e1a1c4af9b5d5d141776064dd5bab3e914268f93a0ecf`.

The all-check `megaease/easeprobe ./daemon` replay timed out at 240 seconds,
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
for the required all-check receipt in the ledger. The separately implemented
shared process-termination correction at `golang/sys` is not yet included in
this ten-correction checkpoint.

Focused resource, lifecyclefacts and architecture tests pass. Focused
lifecyclefacts and resource race tests pass. Repository-wide final validation
and the rest of the active findings are still in progress.
