# Analysis performance investigation, October 6

Status: ongoing; tracked by `gohawk-0mk`, with earlier resource performance
work in `gohawk-rey`. These measurements do not establish that all easy
performance improvements have been exhausted.

## Driver baseline

Baseline production is `b77b59f2` (v0.6.0), Go 1.27.0 linux/amd64 on Xenia,
16 logical processors. All checks are enabled. Production Go source and module
files match the baseline; the shared checkout has unrelated changes. The
standard benchmark analyzes a detached checkout of the recorded revision.

Three warmed self-analysis invocations over 34 packages take 0.816, 1.036,
and 4.137 seconds. These are cache-hit driver measurements, not proof-engine
benchmarks. A separate isolated empty `GOCACHE`, with module downloads already
cached and tool construction excluded, takes 21.003 seconds. An immediate
repeat takes 0.443 seconds. Both exit successfully. The cold run includes Go
build/type work; it does not isolate analyzer work.

The subprocess helper reports peak RSS of 282580 KiB cold and 45540 KiB warm.
This is the operating system's child-process RSS statistic, not the sum of
concurrently resident processes or a memory ceiling.

## Uncached analyzer actions

Using the populated isolated build cache and a previously unused absolute
`-gohawk-timing-file` forces analysis with timing enabled. This invocation
produces 2320 records and takes 13.213 seconds. Summed action durations are:

| Analyzer | Summed seconds |
| --- | ---: |
| lockorder | 4.624 |
| resourcelifetime | 1.214 |
| cancellationownership | 0.903 |
| deferinloop | 0.877 |
| processownership | 0.750 |
| goroutineownership | 0.612 |
| concurrentcapture | 0.554 |
| producerlifecycle | 0.457 |

Each analyzer has 290 records, including dependency and test-package actions.
Actions run concurrently; sums do not partition elapsed wall time. Timing
wraps catalog analyzer runs, excluding prerequisite passes and SSA construction.
Allocation deltas read process-wide statistics and can overlap concurrent work.
The timings identify a profiling candidate, not an exclusive cost attribution.

## Heap-state snapshot copies

A normal lock-analyzer fixture CPU/allocation profile confirms substantial
heap-model work. Of 925 MiB sampled allocated space, region graph construction
accounts for about 236 MiB cumulatively and region-state cloning about 105 MiB.
The profile includes package loading, prerequisites, and checker fact validation;
these cumulative figures overlap and are not production-driver percentages.

Scalar maps in a region-state snapshot previously used a preallocated map plus
`maps.Copy`. Replacing these with `maps.Clone` avoids rehashing each entry while
keeping independent writable maps. Nil inputs retain the previous writable-empty
behavior. Pointee sets and nested contents retain their existing deep-copy path;
region identities remain shared as before. Budgets and proof policy are unchanged.

Three samples of the snapshot microbenchmark give these medians:

| Entries per scalar map | Before ns/op | After ns/op | Bytes / allocations, unchanged |
| --- | ---: | ---: | --- |
| 0 | 377.9 | 276.9 | 496 / 9 |
| 1 | 1219 | 813.0 | 1712 / 14 |
| 16 | 4894 | 2322 | 5624 / 24 |
| 256 | 60130 | 22147 | 79032 / 24 |

The benchmark populates five scalar maps; it is a controlled workload rather
than a measured distribution of production state sizes. Run it with:

```sh
go test ./internal/heapmodel -run '^TestRegionStateScalarClone' \
  -bench '^BenchmarkRegionStateClone$' -benchmem -count=3
```

Alternating three old/new prebuilt lock-analyzer fixture runs with
`GOMAXPROCS=4`, warm caches, and no competing validation gives old wall times
2.026 / 1.991 / 1.870 seconds and new times 1.927 / 1.956 / 1.827 seconds.
All pass. The median difference is about 3.2%; this small sample does not
establish a general end-to-end speedup. Memory observations vary between runs.

## Rejected candidate and remaining work

Applying `maps.Clone` to every pointee set improves larger-set microbenchmarks
but adds heap allocations to small nonescaping copies. A size branch also
prevents inlining (compiler cost 114 versus budget 80), worsening small copies.
Both candidates were removed. The retained snapshot change operates on maps
that already escape inside the returned state.

Further work needs representative fresh-analysis profiles of larger repositories,
prerequisite-pass attribution, and allocation/state-size distributions before
considering additional copying or cache changes. No race tests ran locally.

Raw artifacts are retained under `.build/perf-v060-cold-20261006/`, including
metadata, cold/warm/timing runs, CPU and memory profiles, rejected-candidate
benchmarks, snapshot benchmarks, and alternating fixture comparison results.
The warmed standard harness output is under
`.build/perf-v060-20261006-absolute/`. Absolute output paths are necessary for
that harness because it changes into target checkouts before opening logs.

## Validation of the snapshot change

Focused heap-model, lifecycle, fact-pass and lock-analyzer tests pass. Canonical
generation, module verification, formatting, vet, dead-code, lint and self-analysis
gates pass. The full ordinary test target passes after correcting a test-function
reference in this note; that documentation assertion was the initial verification
failure. Receipts: `verify.log` and `test-final.log` in the artifact directory.

## Lazy snapshots in content joins

A follow-up to `672fd78f` removes the region-state clone at the start of
`mergeContents` when every incoming slot already has explicit original contents.
That join never uses the snapshot. If an incoming slot is missing, the original
full clone and implicit-read order are preserved: those reads can follow backing
copies or wildcard elements whose source this same join changes. Pointee union,
stale flags, widening, and budget behavior retain their existing paths.

The regression test covers backing and wildcard reads and retained stale
incoming evidence. A counterfactual that reads missing slots while applying the
join fails the regression; simply deleting the copy would change the proof.

Three-sample expanded microbenchmark medians, including creation of the mutable
input snapshot in each iteration, are:

| Join / slots | Before ns/op | After ns/op | Before bytes / allocations | After bytes / allocations |
| --- | ---: | ---: | --- | --- |
| Shared / 16 | 12421 | 7995 | 20128 / 106 | 10064 / 53 |
| Shared / 128 | 92951 | 59742 | about 153890 / 554 | about 76945 / 277 |
| Disjoint / 16 | 29079 | 29137 | 40128 / 236 | 40128 / 236 |
| Disjoint / 128 | 347298 | 340683 | about 313667 / 1580 | about 313667 / 1580 |

Shared means every incoming slot is present on the original side; disjoint means
none is. The retained change halves allocations in the shared case and leaves
the disjoint allocation count unchanged. Timing noise remains, and this is not
a measured production distribution of joins. Artifacts:
`merge-expanded-before.txt`, `merge-expanded-lazy.txt`, and
`merge-counterfactual.log` in the original performance artifact directory.

An earlier candidate staged all implicit reads before applying updates instead
of cloning the state. It preserved the tested semantics, but added a staging map
for missing slots. On pinned Caddy, three old/new fresh-analysis pairs took old
61.076 / 84.798 / 60.129 seconds and staged 65.465 / 83.551 / 73.328 seconds.
All three complete JSON outputs had exactly the same two findings, including
related evidence. The staged median was about 20% worse; host load varied, but
there was no basis to retain that candidate as a performance improvement.
The staged candidate was removed. Its initial microbenchmarks overlapped the
Caddy warm-up; their wall times are not controlled comparison evidence.

The Caddy comparison uses revision
`502691f5182123ef30f463d7f132e7c2fe55e2bf`, 48 target packages, baseline
`672fd78f` and prebuilt candidates differing only in the join-copy change. Each
before/after analysis uses a unique timing-file flag to bypass the vet analysis
cache while retaining build caches. There are 8528 timing records per run,
including dependencies and test variants. `GOMAXPROCS=4`, `GOWORK=off`, and
`GOFLAGS=-mod=readonly` match between variants. Runs alternate and exclude
binary construction. Host load is variable; these timings do not establish
a repository-wide speedup. The final lazy-snapshot pair and complete JSON
comparison are retained under `.build/perf-caddy-20261006/lazy/`.

The final lazy-snapshot Caddy pair takes 59.360 seconds before and 56.983 seconds
after, with reported RSS 2506196 and 2339632 KiB respectively. Complete JSON
comparison retains the same two findings, including positions, messages and
related evidence; neither findings nor errors are added. This one pair supports
successful execution and behavioral stability on the pin, not a general speedup.

An additional mixed join benchmark has one missing incoming slot. For 16 slots,
median time is 17018 versus 16595 ns/op; for 128 slots, 119453 versus 122946 ns/op.
Allocation counts and bytes are unchanged (114 allocations / 21232 bytes and
562 allocations / about 154994 bytes). These small timing changes remain noisy;
the extra membership scan is the tradeoff for avoiding unused snapshots.
Artifacts: `merge-mixed-before.txt` and `merge-mixed-lazy.txt`.

For the lazy-snapshot change, canonical generation, module verification,
formatting, vet, dead-code checks, self-analysis, and the full ordinary tests
pass. Lint passes after formatting the new benchmark table as one case per
line. Receipts: `merge-verify.log` (initial lint formatting failure, all other
gates pass) and `merge-lint-final.log` (corrected lint gate). The earlier scalar
snapshot commit `672fd78f` also has passing hosted CI, including targeted races.
The broader performance goal remains open; disabled trace metadata and
prerequisite-pass attribution still need investigation.

## Disabled tracing metadata

A follow-up to `36859343` guards function-identity formatting in lifecycle
summary candidates/decisions, retention budget give-ups, lock-state budget
give-ups, HTTP acquisition boundary events, acquisition-error evidence, and
optional-acquisition evidence. Each guard uses the same
candidate probe that emits the event. The proof runs outside the guard; only
metadata construction and emission move behind selection. Tracing schemas,
reasons, candidate attribution, and analyzer budgets are unchanged.

A normal SSA-function benchmark of disabled lock-budget tracing previously
allocated 48 bytes in three allocations per call, with a three-sample median
of 101 ns. The guarded path has zero allocations and a median of 10.82 ns.
The allocation regression fails on the previous code and passes on the guard;
a separate test checks the enabled unknown budget decision and candidate.
This measures a metadata helper, not a whole-analyzer speedup.

Two immutable binaries analyze a small module containing leaked, closed, and
HEAD HTTP response forms. Complete JSON diagnostics are identical; all 36261
trace records also match as a multiset across target and dependency packages.
This includes 4602 lifecycle summary candidate/decision pairs, 26 lock-state
budget events, and two declined HEAD-client events. A disabled-trace invocation
has the same diagnostics as the enabled invocation. Local HTTP-header-only
trace events and retention budget events are not established by that fixture;
existing focused analyzer tests cover the underlying proofs. The guards retain
the original event construction when those probes are enabled.

Focused lifecycle-fact, lock, resource-lifetime, and tracer tests pass. Artifacts
are in the original performance directory: `trace-budget-before.txt`,
`trace-budget-after.txt`, `trace-budget-counterfactual.txt`, `trace-focused.log`,
`trace-comparison.json`, and the enabled/disabled trace and diagnostic files.

Allocation regressions also cover disabled acquisition-error and optional-acquisition
helpers, and enabled tests pin their evidence reasons, accepted outcomes, function
identity and candidate. Both zero-allocation assertions fail when their previous
unguarded implementations are restored. Receipt: `trace-resource-counterfactual.txt`.

Final verification passes all eight canonical local gates, including the full
ordinary suite; receipt `trace-final-verify.log`. The final binary still matches
all 36261 baseline records and complete JSON diagnostics after adding the two
resource-helper guards; receipt `trace-final-comparison.json`. Earlier passing
receipts are retained but are not substituted for this final source validation.
No local race tests were run. Broader prerequisite-cost attribution remains open.

## Prerequisite profile and escaping pointee copies

Baseline `3870a965` uses the normal resource fixture suite, with a prebuilt test
binary, Go 1.27.0, `GOMAXPROCS=4`, warm caches, and the analyzer package as the
working directory. One CPU/allocation profile passes in 24.590 seconds and
contains 61.13 CPU-seconds. Cumulative sampled CPU includes lifecycle inference
9.94 seconds, heap projection 9.28 seconds, and graph construction 8.17 seconds.
These overlap. Checker fact validation accounts for 31.26 seconds; its source
encodes each inherited fact into two new gob streams and decodes a fresh value.
That is mandatory test-harness determinism checking, not production-driver cost.
A first standalone invocation used the wrong working directory and failed to
find fixtures; its retained log is not a validation or performance receipt.

Sampled allocated space is about 19663 MiB, including about 856 MiB in the
pointee map-copy specialization. Snapshots still copied nested pointee and
pending-defer maps with `pointees.clone`. Unlike temporary query copies, those
maps escape inside the returned snapshot. They now use the existing writable
runtime-clone helper; nested maps remain independent, with shared region
identities and unchanged stale flags. Small temporary copies retain their
previous stack-friendly helper. Nil sets still become writable empty maps.

A benchmark with 16 stored slots gives these three-sample medians:

| Pointees per slot | Before ns/op | After ns/op | Bytes / allocations, unchanged |
| --- | ---: | ---: | --- |
| 0 | 2100 | 1519 | 2456 / 28 |
| 1 | 4745 | 3353 | 7064 / 44 |
| 16 | 14783 | 8247 | 21528 / 76 |
| 32 | 26590 | 26467 | 39960 / 76 |

The candidate profile passes in 20.463 seconds with 58.61 CPU-seconds, but
alternating ordinary fixture repeats do not show an overall speedup: baseline
20.167 / 21.830 / 23.510 seconds; candidate 20.385 / 21.913 / 23.783 seconds.
The median difference is about 0.4% slower, within visible timing noise. This
change is supported by the scoped snapshot benchmark and simpler copying, not
an end-to-end speed claim. Peak memory remains around 2.9 GiB in these fixtures.

A second copy is removed where a join cloned a freshly owned content-query
result. Every path through the private content query already produces an
independent writable set, including recursive and implicit reads. The contract
is documented and tested; a counterfactual returning borrowed explicit contents
fails the ownership regression. Joining now takes ownership of that result
without a second map allocation. Shared slots still use their original union
and widening paths; proof rules, budgets, and query order are unchanged.

Disjoint join benchmarks, with the snapshot change already applied on both
sides, have these three-sample medians:

| Slots | Before ns/op | After ns/op | Before bytes / allocations | After bytes / allocations |
| --- | ---: | ---: | --- | --- |
| 16 | 53196 | 52999 | about 40129 / 236 | about 34753 / 204 |
| 128 | 632201 | 524002 | about 313672 / 1580 | about 270662 / 1324 |

The allocation reduction is the stronger evidence; 16-slot timings are unchanged
and host load varies. Raw profiles, isolated test binaries, microbenchmarks,
ordinary repeats, ownership counterfactual, and focused tests are retained in
`.build/perf-prerequisites-20261006/`. The combined final implementation has a
separate normal full resource-fixture result in `resource-final.tsv`/`.log`.

The combined final normal resource-fixture run passes in 24.257 seconds; it is
one additional sample, not evidence of an overall speedup. Final canonical
verification passes all eight local gates (`final-verify.log`), and `make coverage`
also passes (`coverage.log`), measuring 92.5%. The existing README badge matches
that result. Hosted CI at the preceding trace commit had only a stale badge
failure (92.4% there); current-source coverage is the applicable local receipt.
The performance goal remains open for production prerequisite attribution and
a broad completion review; these fixture profiles do not prove its end state.

## Production prerequisite timing and control-flow allocations

At baseline `fa6f4ed1`, a temporary measurement executable invokes the real CLI
and wraps prerequisite `Run` functions. It records timings without memory-stat
stop-the-world reads. Self-analysis produces 290 records per prerequisite and
passes in 15.921 seconds. Summed prerequisite durations are lifecycle facts
28.854 seconds, SSA construction 3.643, result facts 3.605, concurrency facts
0.830, inspection 0.452, and control-flow construction 0.234. These concurrent
package actions overlap; their sums do not partition wall time. Type loading,
fact import and publication outside `Run` are excluded. The wrapper is an
ignored measurement artifact, not a new product option or source dependency.

An actual unitchecker process profiling lifecycle inference in `math/big`
identifies reachability queues and path-guard key formatting as allocation
contributors. A direct vet target includes test files, so profiling its package
name alone can overwrite profiles from different test variants. Initial such
profiles are retained only as exploratory evidence. The corrected experiment
uses a tiny importing module, skips test-source variants, writes per-process
files, and forces fresh analysis with a unique inactive trace-candidate flag.
A serial experiment uses `GOMAXPROCS=1` to reduce overlapping work. Allocation
profiles still cover process allocations before their snapshot, not exclusively
the lifecycle phase; CPU samples cover the selected pass interval.

Two shared mechanics are improved without changing policy or budgets:

- Path guard keys use a string builder instead of one formatted allocation per
  entry and a joined slice. Key bytes, input order, separators and cutoff
  behavior are unchanged. The regression includes empty identities and literal
  separator/format characters.
- CFG reachability keeps a queue head, reuses a drained owned buffer, and
  compacts consumed entries before growth. Seeds are still cloned, so queue
  reuse cannot mutate SSA successor arrays. Visits, revisits and successor
  edges keep their original order and budget charges; the regression proves
  an exact eight-step cyclic/duplicate-edge case and immutable seed backing.

Three-sample benchmark medians include:

| Workload | Before | After | Before allocations | After allocations |
| --- | ---: | ---: | ---: | ---: |
| 128-block chain | 14194 ns | 13238 ns | 135 | 12 |
| 32 path guards | 3726 ns | 747.4 ns | 66 | 7 |

The chain allocates 10344 versus 9360 bytes; the guard key 2017 versus 1016
bytes. Timing noise remains. An initial queue that retained all consumed
entries cut allocation counts but increased sampled reachability allocation
bytes on the production workload; it was replaced with the compacting queue.
The corrected serial baseline samples about 52 MiB in reachability and 37 MiB
cumulatively in guard keys; the compacting candidate samples about 26 MiB in
reachability and 18.5 MiB in builder writes. Total sampled process allocations
are 330.75 versus 256.11 MiB, a single sample rather than a general memory claim.

All profiles, wrapper source, binaries, exact timing records, consumer module,
primitive benchmarks and focused tests are under `.build/perf-production-20261006/`.
The production path has no checker test-harness fact round-trip stack. No
budgets, search limits, analyzer selection or diagnostic policy were weakened.

Final shared-control-flow source passes every canonical local gate in
`compact-final-verify.log`; the initial attempt failed only import formatting in
the new benchmark file. The reported/accepted HTTP fixture has identical complete
JSON before and after (`diagnostic-comparison.json`), and the importing module
has identical empty diagnostics. `make coverage` passes and measures 92.4%; the
README badge was regenerated using the same pinned tool as hosted CI. Earlier
hosted failures at `3870a965` and `fa6f4ed1` were only the stale badge gate.

The temporary profiler first encountered cached runs with no profile files and
direct vet targets containing test sources. Neither attempt is counted as a
successful isolated profile. The applicable ordinary dependency profiles are
`consumer-*` and `serial-*`; the final compacting queue uses `compact-*`.
Profile matching and variant selection are explicit in the retained wrapper.
The broader completion audit remains outstanding; these measurements identify
and improve two costs rather than prove there are no easy wins left.


## Current cold comparison and state-worklist queue

The latest comparison uses the same archived source target at `10aa0f67`
for both executable versions. The baseline executable is v0.6.0; the candidate
includes the five committed optimizations and state-worklist queue reuse.
Each executable gets its own empty `GOCACHE`. Module downloads are already
cached and executable construction is excluded. Go 1.27.0, all checks, default
parallelism, and complete JSON output are used for both. No benchmark or
validation from this investigation runs concurrently with these controlled
scans. Other host activity remains uncontrolled, so the single pair cannot
establish a reliable speedup.

| Cache state | v0.6.0 | Candidate |
| --- | ---: | ---: |
| Empty build and analysis cache | 21.031 s | 20.190 s |
| Immediate cached repeat | 0.417 s | 0.392 s |
| Build cache populated, analysis forced fresh | 16.033 s | 14.092 s |

These are one sample per state, not a reliable speedup estimate. Both versions
exit successfully with identical complete JSON diagnostics in all three states.
The forced-fresh run uses a previously unused timing-file flag and includes its
instrumentation overhead. Reported child-process peak RSS is 278756 versus
285936 KiB cold; it is not aggregate concurrent-process memory. An earlier
attempt used the mutable working tree and overlapped benchmark work and a test
edit; its numbers are exploratory and excluded from this comparison.

The ordinary `math/big` dependency allocation profile also points to append
allocations in the generic state worklist. That queue previously discarded
capacity at every pop. It now clones the initial slice, keeps a head, reuses a
drained buffer, and compacts consumed entries before growth. Callback successor
slices remain caller-owned. FIFO order, queued revisit charges, expansion keys,
early termination, and key/step cutoff policy are unchanged. A regression
combines branches, duplicate keys and a cycle with exact eight/nine-step budgets,
and checks sentinel storage beyond the initial slice and successor storage.
The ownership regression fails against the previous implementation.

Isolated three-sample benchmark medians, with only this production file overlaid
back to its previous revision for the baseline, are:

| 128-state shape | Before | After | Before bytes / allocations | After bytes / allocations |
| --- | ---: | ---: | ---: | ---: |
| Chain | 14858 ns | 14307 ns | 10368 / 138 | 9360 / 12 |
| Binary tree | 14842 ns | 13893 ns | 11952 / 22 | 10368 / 18 |

A serial ordinary-dependency profile samples about 12.49 versus 6.84 MiB of
flat allocation in the obligation-state worklist instantiation. Whole-process
sampled allocation is 268.14 versus 284.57 MiB, so this experiment does not
establish an overall memory reduction. Allocation profiles include the process
before the lifecycle snapshot and are affected by map order and sampling.
The retained baseline executable predates the source edit; its line listings
against current source are not used as exact line attribution.

Artifacts, archived target, provenance, complete diagnostics, isolated benchmark
outputs, previous-code counterfactual and per-process profiles are under
`.build/perf-current-cold-20261006/`. The completion audit still needs broader
profile and source coverage; this optimization does not prove that easy wins
have been exhausted.

Hosted CI for `10aa0f67` passes all behavior, compatibility and targeted race
jobs. Its coverage badge check fails because hosted measurement generates
92.5% while the locally regenerated badge reads 92.4%; the aggregate test job
fails solely on that prerequisite. This is recorded separately from behavior.


### Remaining completion evidence

The investigation must cover the runner, prerequisites and all eight analyzer
engines before concluding that no straightforward improvement remains.
Current evidence and remaining questions are:

| Scope | Current evidence | Remaining work |
| --- | --- | --- |
| Runner and cache behavior | Controlled cold, cached and forced-fresh pairs; identical diagnostics | Repeat representative larger targets; distinguish build work from pass work |
| Shared prerequisites | Actual unitchecker timing across 290 package actions; lifecycle inference dominates summed prerequisite time | Profile more expensive ordinary package actions, not just `math/big` |
| Heap snapshots and content joins | Ownership regressions, counterfactuals, paired microbenchmarks and real-target measurements | Review remaining pointee-set union/unknown scans and projection work |
| Shared control flow | Reachability and state queues improved with exact-order/budget tests | Review remaining key-builder growth and visited-map costs |
| Reaching-value folds | Production allocation profile identifies `Every`; branches already clone with `maps.Clone` | Preserve branch independence and shared leaf visit semantics while investigating allocation |
| Trace metadata | Disabled/enabled allocation tests and 36261-record complete evidence comparison | Review any remaining unconditional metadata construction in profiled paths |
| Lifecycle type vocabulary | Profile includes repeated resource-type construction; source recreates cleanup slices | Measure rejected and matched lookups before choosing storage/ownership changes |
| Catalog analyzers | Fresh-action baseline timings for all eight; lock/resource fixture profiles and Caddy scans | Inspect the remaining six proof engines against representative uncached production profiles |

These are incomplete audit items, not findings that the remaining costs can
necessarily be removed cheaply. For example, reusing a reaching-value branch's
visited map would change sibling independence, and skipping revisit charges
would change conservative cutoff behavior. Such changes are outside the
performance-only scope even if they benchmark faster.


Final state-worklist source passes all eight canonical `make verify` gates
(`verify.log`) and `make coverage` (`coverage.log`). Local coverage now reads
92.5%; the README badge was regenerated with the pinned hosted-CI generator.
The generated shared-helper reference includes the slice ownership contract.
No local race tests or full precision replay were run; previous-commit hosted
targeted races passed, and current-commit hosted verification will be separate.
