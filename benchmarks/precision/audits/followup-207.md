# Continued review of the 207 remaining false positives

This is an active follow-up, not a completed audit. The
[frozen input](followup-207-input.tsv) contains all 207 reviewed false positives:
140 resource, 23 lock, 17 goroutine, 9 process/cancellation, and 18 other findings.
Original verdicts and the earlier audit records are preserved. A missing model
is work still to do, not a terminal disposition.

The starting implementation is `5074635`. Its immutable all-check binary is
`.build/gohawk-followup78-goroutines-v5`, SHA-256
`5516cad4c83bffd8dca28713df53f8d3d1a463b838c23d302da9e10ddc257419`.
Corrected binaries are built at distinct paths. Replays use the original pins
and the direct CLI with `-enable-all -gohawk-include-tests -json`; failed loads
never count as corrections. Candidate code is only statically analyzed.

## Completed implementation slices

- `fc00ae5`: tuple-returned command factories preserve the same uncertain
  ownership as single-result factories. Mutagen's `pkg/agent/dial.go:118:11`
  no longer reports. This is unknown ownership, not a proven wait, and can miss
  genuine leaks behind a factory. Direct standard constructors remain checked.
- `8e88b7a`: receiving the exact standard context child's Done channel declines
  a loss claim on that path. For a select, only the selected receive arm supplies
  evidence. Gleam's `util/context.go:8:17` no longer reports. Other select arms,
  default arms, unrelated contexts, and signal unregistration remain checked.
- `df6166f`: shared branch feasibility now recognizes identical literal results
  from bounded, source-visible helper bodies. Sonar's Linux `adopt` helper
  always returns nil, so the impossible post-start error return no longer
  produces `internal/spawn/spawn.go:187:12`. Dynamic callees, nonliteral or
  conflicting results, deferred named-result mutations, and exhausted scans
  remain opaque. The scan is nonrecursive and capped at 128 instructions.
- `3178bfb`: eight lock findings corrected through bounded exact branch facts,
  conditional caller release, and explicit imported/loaded-guard uncertainty.
  The [lock checkpoint](followup207-locks.md) retains four reviewed bug controls.
- `758fe4f`: fresh returned-owner inference declines ownership when the exact
  resource is also retained by an external manager, rather than assigning sole
  responsibility to the returned view.
- `b7f5f9b`: direct standard process exits and dominating deferred exits at
  `RunDefers` terminate the shared normal-return proof. The golang/sys
  `unix/syscall_unix_test.go:298` finding disappears in a valid canonical scan;
  the other two resource findings in that scope remain. Conditional defers,
  indirect exit functions, and misleading names do not establish termination.

The analyzer slices have diagnostic and accepted fixtures, proof/trace assertions,
focused tests, focused lint, architecture tests, and race tests. Their canonical
13-package process/cancellation replay retains all seven reviewed bug controls
and both previously corrected sonar fallback sites. Intermediate receipts are
under `.build/followup207-process-v1/` and `.build/followup207-cancel-v1/`.
The shared-flow slice has focused constant/opaque-result tests, lint, race,
architecture and affected-analyzer tests. Its 13-package canonical replay
retains the same seven bug controls and both earlier sonar corrections;
receipts are under `.build/followup207-flow-v1/`. The subsequent extraction of
the literal comparison into a helper is behavior-preserving and passes the
same focused unit suite. The traced Gleam replay produces valid diagnostic JSON
and an unknown decision, not a claim of exact release.

`flow_paths.go` remains cohesive around feasibility and ownership reachability
despite exceeding the file-size review threshold: this change extends the
existing branch-literal decision point, with no separate path or proof engine.

## Work in progress

Resource acquisition/retention, goroutine transport ownership, carried lock
guards, and further ownership gaps are being investigated and
replayed separately. Intermediate suppression is not counted as a verified
correction until relevant genuine-bug controls also pass. No check has been
retired, disabled, or demoted.

Five singleton-loop cases are pending a specific policy decision: the repository
currently prohibits loop-count proofs as false-positive fixes. Approval has
been requested for a narrowly bounded shared cardinality model; other families
continue meanwhile.

## Verified follow-up checkpoint

The family replays now verify 40 corrections from the frozen input: 18 resource,
10 lock, 8 goroutine, and 4 process/cancellation findings. The remaining 167 are
still open. All 140 resource sites now have successful paired replays across
83 package scopes; one is absent under the baseline profile and is not counted
as a correction. Absence after a failed load is never counted. See the family ledgers:

- [Resources](followup207-resources.md): 66 bug controls retained.
- [Locks](followup207-locks.md): four bug controls retained.
- [Goroutines](followup207-goroutines.md): 56 bug controls retained.
- [Process/cancellation](followup207-process-cancel.md): seven bug controls retained.

The combined `make verify` gate passes; dependent shared flow, goroutine, and
process changes are committed together as `511ed2b`. Newly exposed goroutine findings were also reviewed: three real
bugs remain reported, and ten false positives were corrected. The 109-site
goroutine ledger preserves those reviews alongside all original labels.

### Rejected intermediate candidates

The broader goroutine replay found that constant nil-branch feasibility removes
the Stargz `store/manager.go:193` report. Its earlier report depended on an
impossible `result != nil` return; the remaining mixed select was incorrectly
credited as a join even on timeout/error/result arms. This is a genuine bug
control, not a false positive to relabel. Case-local select ownership restores
the genuine report. The final 66-scope goroutine replay retains every original
bug control, including Stargz and controls lost by other rejected intermediate
candidates. Cancellation and transport uncertainty do not prove worker completion.

An intermediate integer-state extension also caused pathological lock-flow
expansion in `image/png`. Those runaway/timed-out replays are invalid, not
corrections. A no-acquisition relevance gate and transactional state budget are
being verified: exhausted functions must publish neither diagnostics nor partial
order edges. The corrected isolated scan took 1.60 seconds and about 144 MB;
all 20 canonical lock scopes subsequently completed successfully. The final
binding-only replay produced exactly the same lock diagnostics in every scope.
