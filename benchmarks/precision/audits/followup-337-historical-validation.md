# Historical precision gate follow-up

These comparisons distinguish historical expectation drift from changes introduced
by the 337-site follow-up. Original cohort labels and expected counts are not
rewritten. The cumulative historical gate remains failing; this is not a claim
that the full historical suite passed. Completed rounds were 2, 3, 4, 5, 6, 8,
10 and 12. Rounds 7, 14 and 16 were interrupted while prioritizing baseline
comparisons and the new cohort; later rounds were not run to completion.

## Process and census comparisons

# Historical cohort gate assessment

Source/history review and subsequent pinned static baseline comparisons for
the followup-337 run. No historical labels or expected counts were changed.
Candidate tests and applications were never executed. Where attribution rests
on source/history rather than a baseline replay, that limitation is explicit.

## Round 4: agent-message-queue processownership lost label

- Repository pin: `avivsinai/agent-message-queue@c8837fbc998b64216707a5e8cb525f56d4889a80`.
- Label: `internal/update/update.go:637:9`, `processownership`, legacy blank check ID, true_positive, provenance unknown.
- Final log: `.build/followup337-cohorts/round-4.log` reports this lost TP.
- Pinned source `scheduleWindowsReplace` lines 634–638 constructs `exec.Command("cmd.exe", "/C", command)` and immediately `return cmd.Start()`. There are no later command uses, callback handoffs, loaded owner fields, or worker goroutines. Its caller invokes it only on the Windows replacement path after Rename fails; the command delays and moves the replacement binary.
- Classification: **pre-existing retired-audit expectation, not caused by followup-337 worker changes**, established by source/history rather than a fresh baseline candidate replay. Commit `fa957eb30c8645dfbf9c31960b10f1eb59428495` (Retire processownership detached audit) removed reporting from the `commandUnusedAfterStart` branch before baseline `c9b65a6`. The exact same no-report branch remains in immutable baseline source. It excludes Start's own returned error from handle-use evidence. This source pattern therefore belongs to the explicitly retired detached-launch audit, not the retained partial-wait check.
- The followup-337 changes in `18cb072` add command field identity resolution and callback/no-return-worker uncertainty; none changes the plain direct-command/return-Start shape here. We did not rewrite the historical TP label as FP: the user-approved historical retirement and legacy blank check ID need an explicit cohort-maintenance decision rather than silently forcing the gate to pass.
- Graph verification: Tier 2 project `home-james-scratch-gohawk`, generation `2026-09-22T11:46:33Z`; exact processownership analyzer.go and ownership.go paths had clean best-effort metadata coverage, followed by current and baseline source reads. External candidate checkout was read directly, not indexed.

## Round 10: label count mismatch

- Final log: `.build/followup337-cohorts/round-10.log` stops before scanning: labels.csv has five labels, labels.expected says six.
- Classification: **pre-existing cohort bookkeeping defect, conclusively present at immutable baseline `c9b65a6`**. `git show c9b65a6:benchmarks/precision/round-10/labels.csv` contains exactly the current five labels; its labels.expected is already `6`.
- Introducing history: `f50a5669ed9cf67927bce305b71f3a90484bfe4e` (Respect readiness and bounded goroutine lifecycle ownership) deleted the `moov-io/rtp20022` goroutineownership/unjoined TP at `pkg/rtp/restrictions_test.go:34:4` but did not change round-10/labels.expected. Git ancestry confirms this commit precedes c9b65a6.
- This is not evidence that a current analyzer lost a sixth site: the cohort guard aborts before candidate analysis. Leave both files unchanged during this follow-up; reconcile the earlier deliberate label removal and count through an explicit separate maintenance decision.

## Other observed cohort status (not reassessed here)

- Round 2: all 53 scannable labels pass (23 FP absent, 30 TP present); zero new findings. Four repositories blocked by missing embedded assets or a missing internal module. Historical 179-gone drift was not deep-audited because it includes prior analyzer retirements.
- Round 4 also reports resource/deferinloop lost labels; they are outside this worker's assigned triage scope.
- Round 6 reports resource/deferinloop lost labels and one new lockorder finding in partially unscannable shelley; these are outside this worker's assigned triage scope. They must not be considered resolved by the two pre-existing issues above.

## Round 3: all nine new-census sites predate this follow-up

All nine sites called new findings by the historical round-3 census are present in both immutable baseline `c9b65a6` and final `5a56fb4`. They are not newly introduced by these changes. This conclusion is verified by six sequential static package replays (one process at a time), not inferred from current source alone.

| Pinned repository | Scoped package | Historical new positions | Baseline/final diagnostics |
| --- | --- | --- | --- |
| erpc/erpc@2b7e807d7d147422cf47c473153eaf9979afdcc9 | clients | http_json_rpc_client.go:388:3, 389:3, 410:3, 427:3, 480:3, 488:3 | 12 / 12, identical |
| pancsta/asyncmachine-go@cce9b31145cb07c1262ac0c71a696222b0119b75 | pkg/machine | machine.go:2157:21, 2807:28 | 11 / 11, identical |
| gumieri/nenya@efae2b7519f8ee292dce3b5a86d509aa5a73b257 | internal/proxy | retry.go:615:15 | 1 / 1, identical |

The historical census counts distinct analyzer/position pairs: erpc's two locks generate separate diagnostic messages on several return positions, and pancsta has two lock-pair messages at 2157. Full analyzer/category/position/message sets, not just the nine deduplicated locations, are equal between baseline and final, with zero additions or removals.

Each checkout HEAD was asserted equal to the repository pin. All six commands exited zero, produced one valid JSON analysis payload, and had no analyzer errors. Replays included tests, used read-only module mode with workspace disabled, and ran no candidate test binaries, applications or generators. Lockorder explicitly enabled missing-release, recursive-acquire, contradictory-order, read-lock-write, mismatched-release and discarded-trylock; nenya explicitly enabled resourcelifetime/missing-release. Thus opt-in checks were not silently omitted. The broader pancsta repository remains partially unscannable in its scripts module, but this exact pkg/machine replay was successful.

Binary SHA256s: baseline `33743c44dfe1f68cbe8dc146989dbe76c0ab02bda55fa8f030a10e32861dfa02`; final `3da9ce166dfd8dc72c3225dc1229298040ae27b04a394c49b2edcf7c3ca772a7`. Reproducer: `.build/followup337-cohort-new-replay.py`. Receipts and raw logs: `.build/followup337-cohort-new-replays/{erpc__erpc,pancsta__asyncmachine-go,gumieri__nenya}-{baseline,final}.{json,log}`.

This establishes old census drift, not whether each old finding is a true or false positive. No labels, census baselines or production source were changed. The eight lost round-3 labels are separate questions and are not resolved by this new-finding comparison.

## Round 3: kubernaut missing-Wait is an actual current recall regression

- Pin: `jordigilh/kubernaut@528b4f7080bf3522c0fa60f1ce87e48dcbcfe4bb`.
- Site: `docs/spikes/multi-cluster-mcp-gateway/spike-s15-fmc-multiformat-parse/e2e_spike_test.go:35:12`, processownership/missing-wait, historical true_positive confirmed at 9613f2d.
- Validated comparison: baseline `c9b65a6` reports the site; final `5a56fb4` does not. Both exact-package static vet runs exited zero with valid JSON and no analyzer errors, pinned HEAD asserted and tests explicitly included. The neighboring `main.go:40:12` missing-wait diagnostic remains present in both runs; baseline has two diagnostics and final one.
- Source: startMCPServer creates an exec.CommandContext with stdin/stdout pipes, starts it, and registers `t.Cleanup(func() { cmd.Process.Kill() })`. It later registers `session.Close` and returns an MCP session over the pipes. There is no Wait in the command cleanup callback; Kill alone does not reap the command. The original TP remains supported by this source and must not be silently relabelled FP.
- Attribution: the new possibleWaitHandoff boundary treats an exact command capture supplied to an opaque callback runner as uncertain ownership. testing.T.Cleanup is opaque at this analysis boundary and receives a closure capturing cmd, so the new general rule suppresses this real missing-Wait case even though the visible callback only kills. This is a coverage loss caused by the current worker change, not the previously retired detached-launch audit or old census drift.
- Parent decision: **accepted false negative**, the sixth measured lost TP across the parent's scoped and historical controls. The opaque callback-consumer boundary intentionally makes ownership unknown; do not add a testing.Cleanup naming/lifecycle exception or demand guaranteed Wait as a substitute for that policy. This does not resolve the candidate bug: Kill still does not reap. Preserve the original historical TP and report the measured recall loss explicitly. No production or label edits were made during this diagnostic task.
- Receipts: `.build/followup337-cohort-new-replays/jordigilh__kubernaut-{baseline,final}.{json,log}`. Reproducer: `python3 .build/followup337-cohort-new-replay.py kubernaut`. Binary hashes match the baseline/final hashes recorded above; all-check broadening was unnecessary because the exact missing-wait check was enabled in both runs.

## Deferred cleanup comparisons

# Historical deferinloop gate failures: rounds4 and6

Read-only source assessment. No candidate process, test, generator, or application
was run; no historical label was edited. The six failures should **not** be
converted to false positives to make the gate pass.

## Observed failures and pins

`.build/followup337-cohorts/round-4.log` loses five gatewayd labels, whose original
confirmation provenance is blank. `round-6.log` loses one Audnex label last
confirmed at `9613f2d` on2026-09-06. Both repositories have reviewed source at
the expected full HEAD:

- gatewayd-io/gatewayd: `bb731040d1f5cd848599024d768f2eea50389638`, checkout
  `.build/followup337-cohorts/checkouts/round-4/gatewayd-io__gatewayd`.
- drallgood/audiobookshelf-hardcover-sync:
  `60106640df841887c584f9cee087366c7ff92d2a`, checkout
  `.build/followup337-cohorts/checkouts/round-6/drallgood__audiobookshelf-hardcover-sync`.

Other unscannable repositories and unrelated lost checks in those logs are not
part of this assessment.

| Site | Source judgment | Why this is not collective resource ownership |
| --- | --- | --- |
| gatewayd `cmd/gatewayd_app.go:714:5` | Retain true-positive cleanup-lifetime hazard | A new timeout context wraps each OnNewClient hook. The hook completes before the next client iteration; only clients are put in the pool, not cancel functions or timeout-context owners. Cancellation waits until the outer setup function returns. |
| gatewayd `cmd/gatewayd_app.go:771:4` | Retain true-positive cleanup-lifetime hazard | OnNewPool receives a fresh per-hook timeout. Its work completes within the configuration-block iteration; no collection needs this timeout after that iteration. |
| gatewayd `cmd/gatewayd_app.go:830:4` | Retain true-positive cleanup-lifetime hazard | The newly created proxy receives runCtx, not this later pluginTimeoutCtx. The latter covers only OnNewProxy and is needlessly retained across later proxy iterations. |
| gatewayd `cmd/gatewayd_app.go:906:3` | Retain true-positive cleanup-lifetime hazard | The server receives runCtx. A distinct later timeout covers OnNewServer; deferring cancel to createServers return extends its lifetime across the server loop. |
| gatewayd `metrics/merger.go:138:3` | Retain true-positive cleanup-lifetime hazard | Each request response is read into independent bytes by io.ReadAll at140. Only those bytes enter pluginMetrics at146; the response body itself is not retained for post-loop use. Its explicit Close is deferred across subsequent plugin iterations. EOF may make the practical cost smaller; this is not evidence that the collection owns the body or a claim that exhaustion is inevitable. |
| audiobookshelf `internal/api/audnex/client.go:183:3` | Retain true-positive cleanup-lifetime hazard | A5xx response takes continue at196 without reading or closing its body. Subsequent attempts and backoff run before deferred bodies close on function exit. Three retries bound accumulation but do not establish timely per-attempt cleanup. The final Book value does not own any earlier response body. |

Gatewayd `plugin/plugin_registry.go:259–335` supports the distinction: Run
creates an inherited child context, defers that child's cancel, and calls hooks
synchronously. Cancelling the child does not cancel the caller's parent timeout
timer. These reports concern avoidably delayed cleanup, not a claim of an
unbounded leak or an inevitably failing program.

## Did the new before-defer aggregate-store rule cause the disappearance?

The source evidence points to **pre-existing coverage gaps**, not a collective-
ownership change. Runtime/binary attribution still needs a baseline replay when
a candidate slot is free; none was launched during this read-only task.

1. **Four cancel-function sites:** `deferredObligation` in both `c9b65a6` and
   finalsource `5a56fb4` starts with `ssaflow.CallReceiver` and immediately
   rejects a nil receiver. A direct `cancel()` function invocation has no
   receiver. Consequently these sites do not reach `resourceLiveAtNextIteration`,
   where the new pre-defer-store rule lives. The current narrow obligation
   finder predates this follow-up and does not handle this cancellation shape.

2. **Two response-body sites:** the candidate receiver is `io.ReadCloser`, not
   `*http.Response`. `lifecyclefacts.ResourceCleanup` only matches its named
   concrete resource vocabulary; it does not call the separate interface-aware
   `typeCleanup`. The fallback needs a proved returned owner with a matching
   release method; `http.Response` has no `Close` method. This is another
   unchanged acquisition-boundary mismatch. No body is stored into an aggregate
   before the defer in either source. Later map entries carry decoded bytes or
   scalar logging fields, not ownership of the response body.

The exact `git diff c9b65a6..5a56fb4 -- internal/analyzers/resources/deferinloop`
contains only the aggregate-store changes to flow.go and the new collection
fixtures. The obligation finder is unchanged. Changes to shared infrastructure
elsewhere can affect indirect evidence, so this source comparison should not
be represented as a completed baseline differential replay.

## Recommended handling

- Keep all six source judgments as genuine hazard examples. They are not
  newfound false positives and should not be relabelled merely because they
  no longer report.
- Run the frozen `c9b65a6` binary with explicit
  `-enable-checks=deferinloop/cleanup-lifetime` on gatewayd `./cmd ./metrics`
  and audiobookshelf `./internal/api/audnex` when a slot is available. Compare
  against finalsource output before attributing a regression to this patch.
- If baseline is also silent, record historical drift / accepted false-negative
  coverage explicitly; do not count these as preserved true-positive controls.
- If baseline reports either HTTP-body site, inspect its actual SSA and
  classifier trace before adjusting the new aggregate rule. Do not presume a
  store of decoded bytes transfers the original response body.
- Restoring cancel-function or response-body obligations is a separate bounded
  acquisition-model change with diagnostic and accepted fixtures, not a reason
  to remove correct collective-lifetime uncertainty from pmtiles.

Graph evidence was used for the relevant production symbols, with clean
best-effort coverage of `deferinloop/{obligation,flow}.go`,
`ssaflow/{call_metadata,store_ownership}.go`, and `lifecyclefacts/fields.go`,
generation2026-09-22T11:46:33Z. External checkout source was read directly
because those directories are outside indexed coverage.

## Completed baseline/final differential

After the initial read-only assessment, root authorized one candidate-process
slot for exact static replay. The sequential replay completed successfully.

| Pinned package scope | Sites reviewed | Baseline c9b65a6 | Final5a56fb4 |
| --- | --- | --- | --- |
| gatewayd `cmd` | 714,771,830,906 | All four absent; package loaded | All four absent; package loaded |
| gatewayd `metrics` | merger.go138 | Absent; package loaded | Absent; package loaded |
| audiobookshelf `internal/api/audnex` | client.go183 | Absent; package loaded | Absent; package loaded |

**Definite attribution: all six absences predate followup337.** They are
historical coverage drift relative to the old labels, not regressions caused
by the new before-defer-store rule. This does not change their source-level
true-positive hazard judgment and does not count them as preserved controls.

All six commands exited0 with no analysis/loader errors. Each command explicitly
enabled `deferinloop/cleanup-lifetime` and test files. Full source pins and binary
hashes were asserted before scanning; modules were readonly, CGO_ENABLED=0,
GOWORK=off, GOTOOLCHAIN=local. No candidate tests, generators, or applications ran.

Complete diagnostic sets were identical for each paired scope. Gatewayd `cmd`
still reports `cmd/plugin_install.go:309:4` and `:409:4` in both binaries;
`metrics` and Audnex report no deferinloop diagnostics in either binary.

Reproducer: `.build/followup337-cohort-locks-replay.py`.
Immutable receipts and full logs:
`.build/followup337-cohort-locks-replay/{baseline,final}-*.{json,log}`.

- Baseline SHA256:
  `33743c44dfe1f68cbe8dc146989dbe76c0ab02bda55fa8f030a10e32861dfa02`.
- Final SHA256:
  `3da9ce166dfd8dc72c3225dc1229298040ae27b04a394c49b2edcf7c3ca772a7`.

Historical labels and production code remain untouched. The six-command replay
has finished and its candidate slot is free.

## Additional round3 differential: erpc and topf

Three more historical deferinloop labels were source-reviewed and replayed
sequentially under the same explicit-check profile and immutable binary hashes.
Their old confirmations were `9613f2d` on2026-09-06.

| Repository and full pin | Site | Source judgment | Baseline / final |
| --- | --- | --- | --- |
| erpc/erpc `2b7e807d7d147422cf47c473153eaf9979afdcc9` | `thirdparty/chainstack.go:277:3` | Genuine per-page response-body cleanup hazard: successful decoding appends independent decoded nodes and advances nextURL; bodies are not required by the returned collection. | Absent / absent; both package loads succeeded |
| erpc/erpc `2b7e807d7d147422cf47c473153eaf9979afdcc9` | `thirdparty/quicknode.go:432:3` | Genuine per-page response-body cleanup hazard: decoded endpoints are appended, offset advances, and the response body remains deferred until pagination returns. | Absent / absent; both package loads succeeded |
| postfinance/topf `b9a65ba4f365ebad06884ee3694dcda985c28027` | `internal/cmd/reset/reset.go:73:3` | Genuine per-node client cleanup hazard: Node.Client constructs a fresh authenticated/insecure Talos client; resetNodes retains only Node values, not these clients. Their Close calls wait through subsequent node resets and optional maintenance waiting. | Absent / absent; both package loads succeeded |

The erpc receiver is again response.Body (`io.ReadCloser`), outside the current
direct concrete resource vocabulary. Topf additionally requires an inferred
ownership contract through Node.Client and the external Talos client constructor;
a Close method name by itself is deliberately insufficient for the current
obligation finder. Neither source contains a collective lifetime requiring the
reported handles to survive their respective iterations.

All four static commands exited0 without loader/analyzer errors. Exact repository
HEADs and binary SHA256 values were asserted. Full deferinloop diagnostic sets
are identical between baseline and final for both package scopes.

Reproducer: `.build/followup337-cohort-locks-round3-replay.py`.
Receipts: `.build/followup337-cohort-locks-round3-replay/*.json` and adjacent logs.

**Combined result: all nine historical deferinloop losses across rounds3,4,6
already occur in baseline c9b65a6. None is introduced by followup337.** Their
source judgments remain genuine cleanup hazards, not collective-ownership
false positives. No code or historical label was changed; the final bounded
replay is complete and its candidate slot is free.

## Resource comparisons

# Historical resource control reassessment (read-only)

No labels or production code changed. No candidate test, application or generator
was executed. Initial reassessment was source-only; subsequently authorized
static scoped package replays are recorded below. Source was read directly from the existing pinned
checkouts; each revision below was checked using git rev-parse. Baseline statements
below distinguish source-policy evidence from the subsequently appended replay
results. Statements about replay being needed describe the initial assessment.

## InditexTech/redkey-operator — retained true-positive assessment

- Revision: e75eeeb977f30ce8738609eb0c2d0b23b15dcc1b.
- Site: test/utils/utils_test.go:69:18, TestUncommentCode.
- Creates a real temporary file. The deferred callback calls os.Remove on its
  name, not tmpFile.Close. All subsequent writes/reads use path-based helpers.
  Unlinking a pathname does not release the still-open descriptor. The original
  true-positive label remains justified; do not relabel it merely because the
  diagnostic disappeared.
- The new SQL-parent and paired-error-helper rules do not match this source.
  The new distinct aggregate-wrapper boundary also has no obvious resource
  aggregate at this site. A pre-existing broad data-dependence/callback boundary
  involving tmpFile.Name and the deferred Remove is plausible, but this is only
  a hypothesis. A baseline scoped replay plus authoritative trace is needed to
  attribute the loss; do not claim it predates this follow-up yet.

## ariga/atlas-operator — historical contract label needs review

- Revision: c6c04e312bbf4f564f7221f434f22db5cf8c1e7c.
- Site: internal/controller/atlasmigration_controller.go:993:12,
  extractDirFromSecret.
- Creates gzip.NewReader over bytes.NewReader(tarball), then passes it to
  migrate.UnarchiveDirFrom. There is no local reader Close, but the gzip reader
  does not own an OS resource or its input. Missing gzip-reader Close is not by
  itself the resource-lifetime defect claimed by this historical label.
- The immutable c9b65a6 contracts.go already deliberately excludes compression
  reader constructors, with a rationale that Close neither closes the input nor
  validates a checksum. Thus this mismatch has a concrete pre-existing contract
  explanation and is not evidence that the new SQL/wrapper boundaries removed
  a valid acquisition. Actual baseline diagnostic absence remains unverified
  without a scoped replay. Preserve the historical label pending explicit review.

## teradata-labs/loom — default-policy mismatch on memory writer

- Revision: 9d8c8c672ce22e1d51374bdfb2064fa0d8719968.
- Site: pkg/storage/shared_memory.go:413:8, SharedMemoryStore.compress.
- Creates gzip.NewWriter over a local bytes.Buffer. It calls Write, returns nil
  on a write error, otherwise calls Close before returning buf.Bytes. No output
  is published on the early error path. There is no OS handle owned by this
  gzip writer or its buffer.
- c9b65a6 already contains memoryWriterExempt: local bytes.Buffer/strings.Builder
  compression writers are exempt unless require-memory-writer-close is enabled.
  Therefore default-policy absence is expected from pre-existing source policy;
  it is not attributable to the new resource-wrapper or SQL changes. An actual
  baseline replay is still needed for a runtime receipt and flag confirmation.
- This is at most an opt-in finalization-contract audit, not a default actionable
  missing resource-release defect. Preserve the original label until reviewed;
  do not silently change it to make the cumulative gate green.

## Gate interpretation

Round-4 and round-6 logs record three missing historical true-positive positions,
so those gates remain failed. Two have clear pre-existing policy explanations;
one is a genuine missing-file-close control needing baseline/trace attribution.
None is included in the 337-site follow-up disposition totals.

## Additional round-12 control: uber-go/zap — retained true-positive assessment

- Revision: bb1a55dd13257cf7cbd06b4146674c67ca614dea.
- Site: zapcore/write_syncer_bench_test.go:88:16, BenchmarkWriteSyncer.
- Creates an os.File using os.CreateTemp in b.TempDir, wraps it with AddSync,
  and writes through the captured WriteSyncer in b.RunParallel. Neither this
  function nor its callback closes the file. Removing the temporary directory
  does not close an open descriptor. The original true-positive label is sound.
- AddSync in zapcore/write_syncer.go returns its input when that input implements
  WriteSyncer (as *os.File does), otherwise returns writerWrapper{w}. Its API
  adds Sync, not Close, and supplies no resource-release obligation settlement.
- This differs concretely from the new aggregate-wrapper case: AddSync receives
  the exact file directly, not a variadic/field aggregate containing it. The final
  possibleAggregateWrapper predicate explicitly excludes SameValue input, the
  boundary added for the bufio.Scanner regression. Consequently this source does
  not by itself establish that the new aggregate-wrapper rule caused the loss.
- c9b65a6 already has potentially relevant callback/asynchronous-exposure
  uncertainty in opaqueCall, including ClosureHandsValueToUnreadableCallee and
  ClosureRetainsValue. This benchmark passes a closure capturing an AddSync view
  to RunParallel. That is a plausible pre-existing suppression mechanism, not an
  authoritative explanation without a baseline replay and trace. Shared summary
  changes also cannot be excluded through source inspection alone.
- Round-12 labels record last confirmation at 9613f2d on 2026-09-06, not at the
  current follow-up baseline c9b65a6. Thus a historical loss is established, but
  attribution to this follow-up is not. Keep the true-positive label and record
  the gate failure; do not reclassify the benchmark as accepted merely to pass.

No additional candidate process was run for this fourth assessment.

## Subsequent authorized baseline/final static verification

All eight scoped package invocations completed successfully: exact checkout pins
matched, exit status was zero, diagnostic JSON decoded, and no analyzer errors
were recorded. Each invocation selected only resourcelifetime/missing-release
and included test files, using readonly modules and the local toolchain. These
were static Go vet invocations; candidate tests/applications were not executed.

| Historical position | c9b65a6 baseline | Final contained binary |
|---|---|---|
| redkey test/utils/utils_test.go:69:18 | Absent | Absent |
| zap zapcore/write_syncer_bench_test.go:88:16 | Absent | Absent |
| atlas internal/controller/atlasmigration_controller.go:993:12 | Absent | Absent |
| loom pkg/storage/shared_memory.go:413:8 | Absent | Absent |

Therefore all four historical missing diagnostics **predate the 337-site
follow-up changes**. This establishes version attribution, not the precise
classifier reason for redkey/zap, and does not justify relabeling their genuine
descriptor leaks. Atlas/loom retain the contract-policy qualifications above.

Receipts and raw output: `.build/followup337-historical-resources/receipts.json`
and per-repository variant files in that directory. Immutable binary SHA256s:

- Baseline: 33743c44dfe1f68cbe8dc146989dbe76c0ab02bda55fa8f030a10e32861dfa02.
- Final: 28584c84253a04b1b394e45d36896f3b055d954ab275938ac1c0236bc29a4e59.

No historical labels, replay stamps or production source changed.

## Additional round-3 historical controls

Four additional labels were inspected and compared with the same immutable
baseline/final binaries, exact missing-release check, test inclusion, pinned
revisions and valid-load checks. All eight static invocations passed; all four
positions are absent in both versions. They too predate this follow-up.

| Repository and pin | Position | Source reassessment |
|---|---|---|
| erpc/erpc 2b7e807d7d147422cf47c473153eaf9979afdcc9 | util/gzip_pool_test.go:48:14 | stdlib gzip.NewReader over local bytes.Buffer; baseline already excludes compression reader acquisition. Historical contract label needs review, not automatic relabeling. |
| flowexec/flow 958773d81d410dd71e21460abb77da302617f96c | internal/runner/parallel/parallel.go:190:18 | os.Open(os.DevNull) handed to a shallow-copied task Context through SetIO; Context.Finalize closes its stored stdin, but this task closure does not finalize its copy. Retained owner/lifecycle relation explains uncertainty; whole-program cleanup not established by this bounded review. |
| jordigilh/kubernaut 528b4f7080bf3522c0fa60f1ce87e48dcbcfe4bb | test/infrastructure/kind_cluster_helpers.go:346:20 | Genuine missing-close error path: WriteString failure returns before Close; deferred Remove only unlinks the path. Keep original TP. |
| jordigilh/kubernaut 528b4f7080bf3522c0fa60f1ce87e48dcbcfe4bb | test/infrastructure/workflowexecution_e2e_hybrid.go:1093:18 | Same genuine WriteString-error path before Close; Remove does not close descriptor. Keep original TP. |

Flow's Context.SetIO merely stores stdIn/stdOut; ShallowCopy gives the task its own
mutable stdin field while sharing callbacks. Context.Finalize closes the receiver's
stdin, so root finalization alone does not prove release of the copied task's
replacement devNull. No Finalize call was found within internal/runner. This is
not a whole-program negative claim and does not authorize changing the label.

Receipts: `.build/followup337-historical-resources-round3/receipts.json`, plus raw
stdout/stderr and per-position receipts. No code, labels or stamps changed; no
candidate tests or applications ran. These four are outside the 337-site totals.
