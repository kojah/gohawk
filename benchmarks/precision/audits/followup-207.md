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
