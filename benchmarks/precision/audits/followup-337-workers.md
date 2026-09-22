# Worker lifecycle follow-up: 81 reviewed sites

All 81 assigned original false-positive sites were re-reviewed against their pinned sources. The worker-scoped corrected replay verified 39 absent and retained 42 needing stronger evidence. Final source `5a56fb4` confirmed exactly the same results across 55 successful package scopes using explicit input check IDs; all four real TP controls remain present. The entire diagnostic set is identical to the prior integrated replay: no added or removed diagnostics. Original audit verdicts have not been rewritten.

| Analyzer | Reviewed | Verified absent | Retained needing proof |
| --- | ---: | ---: | ---: |
| cancellationownership | 9 | 4 | 5 |
| goroutineownership | 40 | 16 | 24 |
| processownership | 11 | 5 | 6 |
| producerlifecycle | 21 | 14 | 7 |
| Total | 81 | 39 | 42 |

## Replay provenance

- Frozen baseline: `.build/gohawk-followup337-baseline`, source `c9b65a6`, SHA256 `33743c44dfe1f68cbe8dc146989dbe76c0ab02bda55fa8f030a10e32861dfa02`.
- Corrected worker binary: `.build/gohawk-followup337-workers-v2`, SHA256 `84984c5030402aa2ef8b2890935ee3f44eac64fb7185cfcc84769f1ed2146b31`.
- Prior integrated binary: `.build/gohawk-followup337-integrated`, SHA256 `652c952b7107d763975da61339bd6b7e8ef7d2b387d9fbae01ba3f68af6518d2`. Its replay used `-enable-checks=<exact input check IDs>` instead of analyzer selection; all 55 scopes and all 81 outcomes matched v2.
- Final contained binary: `.build/gohawk-followup337-final-contained`, source `5a56fb4`, SHA256 `3da9ce166dfd8dc72c3225dc1229298040ae27b04a394c49b2edcf7c3ca772a7`. All 55 receipts match this hash, all scopes exited zero without analyzer errors, all 81 site outcomes are unchanged and all four TP controls remain. Comparing analyzer/category/position/message across all emitted diagnostics found zero additions and zero removals against the prior integrated replay. Final per-site receipt paths in the ledger point to `.build/followup337-workers-final-contained/`.
- Sequential static package replay used `go vet -vettool=<binary> -enable=<analyzer> -gohawk-include-tests -json .`, read-only module mode, disabled workspace, local toolchain, CGO disabled. No candidate tests, applications, or generators ran.
- Every one of the 81 original positions was reported by the baseline under the identical scoped profile. Therefore no absent result is attributable to a check being disabled by that profile. Every corrected scope exited successfully without analyzer errors. The parent will still use all checks for the final integration replay.
- Per-site original source evidence, commit-pinned links, outcomes and receipt paths are in `.build/followup337-workers.tsv`. Baseline receipts are under `.build/followup337-workers-baseline/`; corrected receipts under `.build/followup337-workers-v2/`.
- `.build/followup337-workers-labels.csv` contains the 39 scoped verified FP labels plus four surviving real TP controls. `.build/followup337-workers-repositories.tsv` supplies all 21 corresponding repository pins. These inputs deliberately have no final clean-revision confirmation stamp yet.

## Bounded changes and coverage tradeoffs

### Cancellation: parent context snapshot

Four sites (Shopify/ghostferry and three werf/nelm paths) retain a cancellable parent in a captured local cell subsequently assigned the child. Resolve the parent at child creation through existing shared storage mechanics, then use the existing cancellation classifier. This is exact parent provenance, not an inference from context variable names. Detached or ambiguously replaced parents remain outside the proof. Fixtures retain unrelated-parent, conditional-cancel and detached-context diagnostic controls.

### Goroutines: unknown ownership and completion boundaries

Sixteen sites are absent after four local precision changes: factory/registry-origin signals do not prove exclusive caller ownership; returning a projection of the same aggregate can make ownership unknown; a close inside an ongoing worker loop is not necessarily worker completion; exact post-launch local cancellation can cover all caller returns. These decline reports rather than asserting an opaque factory guarantees completion or cancellation joins a worker. Local channel abandonment and unrelated returned channels remain diagnostic controls. Previous diagnostic fixtures whose factory ownership is now intentionally unknown were deleted, with the accepted false-negative gap documented in their headers.

### Processes: exact identity and opaque waiter handoff

Five sites are absent. Existing storage resolution connects a field-loaded command to its returned value owner. An exact command captured by an opaque callback makes the missing-wait claim uncertain. A started local worker with no normal return and a reachable exact Wait also makes the claim uncertain: this uses may-Wait coverage, not vacuous every-return completion. Callbacks capturing another command and workers without Wait remain controls. No shared completion contract was weakened.

### Producers: reject unproven cardinality and caller termination

Fourteen sites are absent. A send in a loop does not establish multiple feasible sends: singleton containers and success guards are counterexamples. If any relevant producer count is unknown, the finite consumer-count proof declines the channel. Continuing or process-terminating callers with no normal return are also outside this finite-return check; this is not a general liveness guarantee. Explicit sequential excess sends and competing workers remain diagnostic controls. The former repeated-loop diagnostic fixture was deleted and its intentional coverage loss documented.

## Retained gaps: 42 sites

These remain reported by the scoped corrected binary. They are not claimed resolved by this assessment, and no check was retired, disabled or demoted.

| Evidence family | Sites | Missing evidence / boundary |
| --- | ---: | --- |
| Transport shutdown completion | 9 | Connect transport close/error behavior to exact worker exit across helpers and callbacks. |
| Service-return feasibility | 7 | Establish which background-context service returns are feasible and whether their caller terminates; no framework/name exemption. |
| Transitive completion | 6 | Prove indirect worker settlement across helper/channel relationships. |
| Process-lifetime signal registration | 3 | Exact command-wide registration and termination contract, not generic main-function exemption. |
| Correlated command identity | 3 | Preserve command/error/branch correlations across merged SSA values. |
| Alternative completion | 2 | Relate distinct completion paths to the same obligation. |
| Process-lifetime worker | 2 | Prove the relevant process boundary and absence of continuing lifecycle use. |
| Cancellation-bound worker | 2 | Establish cancellation propagation to the actual worker without widening unrelated contexts. |
| Stable field guard correlation | 1 | Relate a stable owner field guard to its cleanup branch. |
| Factory or registry signal | 1 | tusd's worker aggregate and returned completion channel still require a stronger relation. |
| Detached process protocol | 1 | Exact protocol transfer, not a detached-process naming convention. |
| Context Done branch | 1 | Prove cancellation state on the selected branch. |
| Pre-start process owner | 1 | Relate an already-created transport owner to the later successful Start obligation. |
| Constant helper error result | 1 | Prove the helper's feasible return rather than trust apparent platform intent. |
| Typed shutdown protocol | 1 | Exact typed protocol ownership/termination relation. |
| Expired timer | 1 | Timer-expiry semantics on this path; no elapsed-time assumption from syntax alone. |

## Positive controls and validation

Four real TP sites remain present in baseline, worker-v2, integrated and final-contained receipts (the integrated/final-contained directories have matching per-scope receipt names, except repository-root scopes use `-.json`):

- Shopify/ghostferry `data_iterator.go:67:3`: configurable Fatal can return after a sorter error, abandoning workers before queue close / Wait. Receipt: `.build/followup337-workers-v2/51-Shopify__ghostferry-goroutineownership-..json`.
- trzsz/trzsz-ssh `tssh/waypipe.go:123:12`: started command with stderr pipe; exit callback only signals, never Waits. Receipt: `.build/followup337-workers-v2/51-trzsz__trzsz-ssh-processownership-tssh.json`.
- saljam/webwormhole `cmd/ww/pipe.go:34:3` and `:42:3`: two terminal unbuffered sends, one receive and normal caller return leave the other producer blocked. Receipt: `.build/followup337-workers-v2/54-saljam__webwormhole-producerlifecycle-cmd_ww.json`.

All four focused analyzer test packages passed together, and goroutineownership passed again after adding the unrelated-returned-channel control. `TestAnalyzerCommentaryCoverage` passed after a rationale comment was added at the cancellation boundary. All touched Go files were formatted and `git diff --check` passed. Parent owns final full-suite, clean binary, all-337 integration replay, regression-round stamping and push.

Implementation commits recorded by parent: `a1d23d2` (goroutine), `c6b3e6f` (producer), `18cb072` (process), `4d7bf27` (cancellation).
