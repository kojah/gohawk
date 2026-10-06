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
| Shared prerequisites | Actual unitchecker timing across 290 package actions; ordinary `math/big`, `runtime`, `reflect`, and `go/types` profiles | Review residual heap projection/map costs and broaden representative target coverage |
| Heap snapshots, joins and queries | Ownership regressions, counterfactuals, paired benchmarks and real-target profiles; repeated union scans removed and unused query map avoided | Review individual-add unknown scans and residual projection work |
| Shared control flow | Reachability/state queues and key capacity improved with exact-order/budget tests | Review remaining guard filtering and visited-map costs |
| Reaching-value folds | Production allocation profile identifies `Every`; branches already clone with `maps.Clone` | Preserve branch independence and shared leaf visit semantics while investigating allocation |
| Trace metadata | Disabled/enabled allocation tests and 36261-record complete evidence comparison | Review any remaining unconditional metadata construction in profiled paths |
| Lifecycle type vocabulary | Eight discarded slices removed; rejected/package lookups allocate zero; matched result ownership and non-call acquisition boundary tested | Lookup overhead addressed; no whole-run improvement established |
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


## Lifecycle type-vocabulary allocation

The standard resource vocabulary previously constructed eight cleanup slices
for every lookup, including a rejected type and a package-origin lookup that
never returned cleanup methods. The vocabulary now stores its one or two
method names in a fixed array. Only a successful `ResourceCleanup` allocates
its own writable result slice. No global mutable vocabulary is introduced.
All eight known types, value/pointer forms, cleanup ordering, caller mutation
independence, wrong-package and wrong-name matches, timers, basic types and
additional pointer layers retain their previous answers.

Three-sample benchmark medians are:

| Lookup | Before | After | Before bytes / allocations | After bytes / allocations |
| --- | ---: | ---: | ---: | ---: |
| Basic type cleanup | 210.6 ns | 35.07 ns | 144 / 8 | 0 / 0 |
| Unknown named type cleanup | 208.6 ns | 49.46 ns | 144 / 8 | 0 / 0 |
| File cleanup | 175.1 ns | 37.61 ns | 144 / 8 | 16 / 1 |
| Transaction cleanup | 209.8 ns | 54.67 ns | 144 / 8 | 32 / 1 |
| File defining package | 180.4 ns | 13.73 ns | 144 / 8 | 0 / 0 |
| Transaction defining package | 228.1 ns | 18.44 ns | 144 / 8 | 0 / 0 |

Acquisition inference also stops at its existing call-result requirement before
looking up the type. A parameter, a load, or a zero-value allocation of a known
resource type never established acquisition evidence; inspecting cleanup first
was unused work. Real SSA fixtures prove those accepted forms still carry a
known resource type but no acquisition. After the vocabulary change alone,
these rejected forms still allocated one 16-byte cleanup slice at about 35 ns;
the early call test reduces them to zero allocations and about 2.6 ns.
This does not broaden or narrow the acquisition contract or spend any new
query budget. Existing positive acquisition and cross-package fixtures remain
the behavior gate for real constructor calls.

The ignored production profiler was expanded to ordinary dependencies
`runtime`, `reflect`, and `go/types`, reached through a tiny importing module.
Each pair selects lifecycle inference in exactly one non-test package process,
uses `GOMAXPROCS=1`, and forces fresh analysis with a distinct inactive trace
candidate flag. The baseline is `d7fda097`; both binaries use the same wrapper,
with only the resource-vocabulary file overlaid back for the baseline. This
profile pair predates the final early call test. Complete JSON diagnostics
match for every pair; the final candidate is separately compared against all
baseline JSON results.

Single sampled process allocation totals are 567.77 versus 568.78 MiB for
runtime, 219.20 versus 187.35 MiB for reflect, and 319.51 versus 288.65 MiB for
go/types. These cover process allocations before the pass snapshot, not only
the selected lifecycle work, and vary with map order and sampling. They do not
establish an overall memory or speed improvement. Runtime and reflect samples
continue to identify state-map cloning, pointee unions and GC work as large
costs; go/types also spends time constructing and interpreting type evidence.
Those operations need a separate semantic review rather than a guessed cache.

The wrapper attempt to wrap catalog roots does not observe their actual runs:
`analyzers.Analyzers` creates fresh root analyzers for the CLI's later selection.
The shared prerequisite objects do survive that construction. Each completed
scan has 81 records for each of the six prerequisites and zero catalog-root
records. These are prerequisite profiles only, not evidence of catalog-profile
coverage. Future catalog profiling must wrap the roots actually submitted to
the driver.

Artifacts are under `.build/perf-vocabulary-20261006/`: before/after primitive
benchmarks, borrowed SSA fixtures, identical-wrapper binaries, the exact source
overlay, all six per-process profiles, prerequisite timings, and complete JSON.
The first final-scan launch preceded binary construction finishing; it failed
to start and supplies no validation evidence. The later successful launch is
the applicable final diagnostic comparison.

Hosted CI at `d7fda097` is green, including coverage and targeted races (run
37398782706). Broader analyzer profiling and guard-key sizing remain open;
there is no completion claim for the overall performance objective.


Final vocabulary and acquisition source passes all eight canonical local gates
in `final-verify.log`. The initial verification attempt passed behavior but
caught an unused test assignment and a missing `tb.Helper`; both were corrected
before the final gate. `make coverage` passes and measures 92.5%, matching the
pinned badge generator without a README change. No local race tests or full
precision replay were run. Previous-commit hosted races are green; hosted
validation of this change will be tracked separately.


The final production binary also scans clean, pinned Caddy revision
`502691f5182123ef30f463d7f132e7c2fe55e2bf` with every check enabled. Its two
complete diagnostics are identical to the retained lazy-copy baseline: no
added or removed findings (`caddy-comparison.json`). Diagnostics produce the
expected nonzero exit; no analyzer error objects appear. This scan overlapped
validation and is diagnostic evidence only, not an elapsed-time benchmark.


## Actual analyzer timing and shared-disk contention

An ignored Go source overlay now wraps `result.invocation.analyzers` immediately
before `unitchecker.Main` in the CLI, after selection. It does not change the
production checkout. This observes the actual root analyzers as well as their
shared prerequisites. An initial self-analysis records 4060 actions: 290 for
each of the eight catalog analyzers and six prerequisites. Complete diagnostics
remain empty. Lifecycle inference sums to 20.852 seconds; lockorder 3.815;
result inference 2.830; SSA construction 2.639. These concurrently running
package actions overlap and are not a wall-time partition. The wrapper excludes
record encoding and output I/O from each duration and uses no memory-stat STW
reads. The overlay, executable, source hash, initial output and exact timing
records are under `.build/perf-actual-analyzers-20261006/`.

The user identified shared-disk contention during this investigation. Repeated
forced-fresh vet actions publish new dependency facts; isolated Go caches,
compiler workspaces, and overlapping verification/coverage amplify the writes.
The recent benchmark/artifact and isolated-cache directories consume about
2.5 GiB. Shared Go build-cache and temp directories measured 299 and 106 GiB;
those totals contain other sessions' work and are not attributed to this task.
No shared cache or another session's job was removed or interrupted.

A current five-second `/proc/PID/io` sample after the Go scan ends has no active
Go writer. It observes 3.254 MiB of writes from the shared Codex server and
2.680 MiB from the Mash experiment. I/O pressure still reports stalls. These
observations do not reconstruct the earlier Caddy write burst or attribute all
system stalls to gohawk; the earlier fresh-analysis workload plausibly
contributed substantial cache and temporary-file writes.

The new `scripts/with-ram-go.sh` places `GOCACHE`, `GOTMPDIR`, and `TMPDIR`
in a reusable private tmpfs workspace. A nonblocking lock serializes use of
that workspace. Go concurrency defaults to two; verification runs one gate at
a time; child commands use reduced CPU priority and idle I/O priority where
available. Output and exit status are preserved. Module downloads and explicit
output paths retain their configured locations. The maintained
[performance workflow](performance-workflow.md) describes reuse and measurement.

Real command-tree I/O receipts using a fresh RAM cache are:

| Target | Physical writes | Physical reads | Exit | Complete diagnostics |
| --- | ---: | ---: | ---: | --- |
| Self | 36 KiB | 0.72 MiB | 0 | Identical, empty |
| Pinned Caddy | 192 KiB | 129.6 MiB | 3 | Identical, two findings |

Linux child `ru_inblock`/`ru_oublock` deltas supply physical I/O counts; tmpfs
writes do not count as disk writes. The first self run takes 39.387 seconds and
Caddy 190.531 seconds. Their cache state, lower concurrency and priority differ
from earlier runs, so these are I/O validation, not evidence of a latency
speedup. The RAM workspace uses about 1.9 GiB of a 32 GiB tmpfs after both jobs;
raw timing and temporary output remain there. Only small receipts, diagnostics
and the aggregate timing summary are copied to `.build/perf-low-io-20261006/`.

Shell syntax and behavior checks cover inherited environment, retained cache,
stdout, exit status, invalid concurrency, mutual exclusion, and rejection of a
disk-backed workspace. The first negative test incorrectly assumed `/tmp` was
disk-backed on this host; the corrected test uses the checkout filesystem and
passes. All Go production source remains at `54b33853`, whose hosted CI is
fully green (37400193884). Existing Go receipts remain applicable; no full
local Go test or coverage rerun is needed for this opt-in shell wrapper and
development documentation.

Further profiling and validation for this investigation use the reusable RAM
workspace, run heavy jobs sequentially, and persist only useful final evidence.
The broader performance completion audit remains open; actual root timing
coverage is now established, while individual proof-engine profiles and
remaining guard-key/heap costs still need review.


## Guard-key capacity and canonical source selection

The remaining guard-key builder growth is avoidable. A budgeted sizing pass
charges exactly the same guard entries, then reserves the encoded byte count
before rendering. Empty guards return immediately without spending allowance.
Each entry still serializes the same identity, separator, equal sign and Boolean
text, in input order. Capacity arithmetic detects integer overflow and disables
the hint instead of wrapping a `Grow` argument; it does not establish proof or
change the rendering policy. Tests cover exact/partial budgets and candidate
pools, empty guards, literal format/separator characters and fitting/overflowing
capacity arithmetic.

Three-sample primitive medians under the RAM workflow are:

| Guard count | Before | After | Before bytes / allocations | After bytes / allocations |
| --- | ---: | ---: | ---: | ---: |
| 0 | 3.129 ns | 1.871 ns | 0 / 0 | 0 / 0 |
| 1 | 59.90 ns | 34.89 ns | 24 / 2 | 16 / 1 |
| 8 | 247.7 ns | 181.2 ns | 248 / 5 | 112 / 1 |
| 32 | 723.3 ns | 606.7 ns | 1016 / 7 | 480 / 1 |

An ordinary `math/big` dependency profile samples 20 versus 11.5 MiB in guard
key allocation. Whole-process sampled allocations are 249.46 versus 270.57 MiB,
so this does not establish an overall memory reduction. Both versions have
identical complete JSON diagnostics. The initial capacity-only candidate slowed
empty keys slightly; the retained early return fixes that case. This profile
pair predates the source-selection shortcut below.

Actual catalog profiling exposes repeated source-file selection work. A
processownership profile on ordinary runtime dependency analysis spends 50 of
its 60 ms of own filtered CPU samples in `SourceSSAFunctions`. File-position
queries and the package-wide test-file census are contributors. The profile
interval also samples other concurrently running analyzer goroutines; unfiltered
samples are not exclusive processownership CPU. `AnalyzeFile` formerly performed
that census before asking whether a vet invocation or canonical-driver marker
already supplied the answer. It now asks those sufficient conditions first.
Generated and test-file exclusions still happen before any driver shortcut.

Driver fixtures preserve ordinary production selection, uncanonical augmented
variants, canonical augmented variants and vet variants, including generated
and test files. A 200-file vet package lookup goes from a three-sample median
10380 ns to 53.33 ns, with zero allocations in both versions. A post-change
processownership CPU interval is too short to collect samples; this is not
proof that its residual work has disappeared. Single runtime action timings
fall across the catalog roots, for example processownership 220.9 to 10.3 ms,
with concurrency and overlapping roots disclosed.

Combined actual-driver self-analysis uses the same current source target for
both executables, a shared warmed RAM build cache, two Go workers, low priority,
all checks and identical Run timing wrappers. Each invocation forces fresh
analysis with a distinct inactive candidate flag. Three alternating pairs are:

| Pair | Before | Candidate |
| --- | ---: | ---: |
| 1 | 22.263 s | 21.540 s |
| 2 | 26.866 s | 25.268 s |
| 3 | 25.301 s | 23.950 s |

The medians are 25.301 and 23.950 seconds. All six invocations exit successfully
with identical empty complete diagnostics. Each emits 4060 action records,
290 for each of the eight catalog analyzers and six prerequisites. Host load
varies; these matched self-analysis results do not establish a general speedup
on other repositories. Median summed catalog timings include processownership
0.294 to 0.062 seconds, cancellationownership 0.352 to 0.106, goroutineownership
0.265 to 0.044, and resourcelifetime 0.533 to 0.270; these sums overlap and do
not partition wall time.

Raw binaries, exact overlays, benchmarks, CPU/allocation profiles, paired action
records and diagnostics remain in
`/dev/shm/gohawk-perf-01a0f86c/guard-key/`. The first runtime profile launch used
a relative wrapper path from the importing module and failed to start; only the
corrected absolute-path launch supplies profiling evidence. Final validation
uses the RAM workflow with heavy jobs sequential. The first local gate passed
behavior but caught redundant test parentheses in the canonical formatter;
they were corrected before the final gate.

The completion audit remains open: actual root timing and a source-selection
profile now cover more of the common cost, but residual lock-analysis, heap
projection, pointee-set and visited-map costs still need bounded review.


Final source passes all eight canonical `make verify` gates in the RAM
workspace, plus `make coverage` at 92.5%. Pinned Caddy's two complete diagnostics
are identical to the previous baseline, with its expected exit 3. Only small
benchmarks, paired summaries, diagnostic comparisons and the final gate receipt
are copied to `.build/perf-key-source-20261006/`; raw profiles and caches stay
in RAM. The badge helper's first attempt while Caddy owned the workspace was
rejected by the serialization lock; it is rerun after that job ends. No local
race tests or full precision replay were run. Hosted CI at the parent commit
`0a9f2f78` is fully green; current-commit hosted results remain separate.

## Pointee union: retain the unknown answer during a merge

The next actual-driver lockorder profile selects ordinary certmagic dependency
analysis in pinned Caddy. The complete scan's JSON matches the previous two
findings; stderr is empty. Its process CPU profile contains 1.11 seconds of
samples, of which 270 ms have the selected lockorder Run on their stack. Other
analyzers and GC also run during the selected interval. The allocation snapshot
contains 519.40 MiB process-wide and 111.99 MiB with selected Run stacks; these
are allocations since process startup, not allocations exclusively during Run.
Heap snapshots, pointee additions and heap queries remain visible contributors.
Raw profiles and action records remain in the RAM workspace's `lock-audit/`.

`pointees.union` previously called `add` for every member, which scanned the
destination for unknown before each known addition. The merge now scans once,
retains that answer, and changes it when an incoming unknown absorbs the set.
Unknown additions still delegate to `add`, preserving its clearing and stale
flag behavior. Empty sources return without scanning. There are no proof,
budget, traversal-order or diagnostic policy changes.

Three-sample primitive benchmark medians use the same warmed RAM Go cache,
two workers and reduced priority:

| Merge | Before | Candidate |
| --- | ---: | ---: |
| Empty | 4.92 ns | 2.38 ns |
| One known target | 112.9 ns | 111.3 ns |
| Eight known targets | 683.3 ns | 378.2 ns |
| Thirty-two known targets | 9362 ns | 1667 ns |
| Already unknown | 68.91 ns | 67.66 ns |

All cases allocate zero bytes. These are repeated merges of overlapping sets,
not a whole-program speed claim. Focused tests cover stale overlap, retaining
stale evidence, unknown absorption in either direction, independent source
ownership, empty input and self unions. The full heapmodel suite also passes.
Benchmark output and validation receipts remain in the RAM workspace's
`pointee-union/`; only small final receipts are retained on disk. The broader
completion audit remains open.

Final `make verify` passes all eight local gates using the serialized RAM
workspace. The coverage target's exact commands, with only its two output
paths redirected into RAM, pass at 92.5%; the README badge remains accurate.
The 41 MiB coverage profile stays in RAM. Self-analysis with all checks reports
nothing. The Caddy comparison above is the pre-change profile run; this change
does not claim a measured Caddy speedup. No local race tests or full precision
replay were run. Hosted CI for parent `20973ecb` is fully green; the new commit's
hosted checks remain separate.

## Content queries: allocate a merge set only when needed

The certmagic lock profile also records allocations in `contentFollowing` and
`unwritten`. An ordinary read with no stored contents allocated an empty merge
map, discarded it, then returned the independently owned map from `unwritten`.
The merge map is now created only when stored evidence contributes or a dynamic
index query needs to union possible elements. Queries still return independent,
writable sets; stored sets and implicit answers are never shared with callers.
Unknown absorption, stale flags, cycle detection, hop limits, backing snapshots,
constant/wildcard selection and the unwritten alternative remain unchanged.

The primitive benchmark confirms an unwritten local query falls from 384 bytes
and three allocations to 336 bytes and two allocations. Written queries remain
336 bytes and two allocations. The first candidate run included the full heap
test suite, unlike the baseline, so its timing is not comparable. Matching
benchmark-only runs use the same warmed RAM cache, two workers and reduced
priority. Three-sample unwritten medians are 135.2 ns before, 145.1 ns candidate,
then 135.6 ns in a baseline repeat; the individual ranges overlap. Written
medians are 171.7, 173.9 and 178.8 ns. The retained claim is fewer allocations,
not lower latency or a whole-program speedup.

The full heapmodel tests pass, including existing writable-result and backing
cycle/depth regressions. New index-query cases preserve a constant read from
wildcard storage, a dynamic read's unwritten alternative, unknown absorption,
empty stored sets and independent results after mutation. Original source for
the baseline repeat is supplied through a RAM-backed Go overlay; both versions
use the same benchmark code. Raw results remain in the RAM workspace's
`content-query/`. The broader performance audit remains open.

The first local completion gate passes behavior but catches excessive nesting
in the lazy-allocation index branches. Flattening dynamic and constant reads
preserves the selection policy: the wildcard step always has the index prefix.
The final source passes all eight `make verify` gates. A final benchmark-only
repeat reports medians of 121.6 ns unwritten and 156.5 ns written, with the same
allocation counts as the earlier candidate. Variation across the recorded
trials still prevents a general latency claim. Hosted CI for parent `3046ef6b`
is fully green, including its targeted race job; the final source has not run
local race tests.

Coverage also passes at 92.5%, using the canonical target's commands with its
profile and summary paths redirected into RAM; the README badge stays accurate.
Only benchmark output, the final gate log and the small coverage summary are
copied to `.build/perf-content-query-20261006/`. The large profile and cache stay
in RAM. No full precision replay or new whole-corpus timing comparison was run.

A bounded projection source review confirms `orderedSlots` already reserves
capacity, and its ordering controls which slots publication bounds retain.
`resultContents` materializes otherwise implicit snapshot fields and
`copyContent` rejects post-write placeholders, so those steps cannot simply be
skipped. Two remaining measurement candidates are reflected sorting in
`orderedSlots` and the split used only to count path depth in `boundedSlot`.
Their source shape alone does not establish a worthwhile improvement.

## Projection: sort slots directly and count separators

Focused benchmarks confirm two small projection improvements. `orderedSlots`
now uses `slices.SortFunc` instead of reflected `sort.Slice`, preserving the
region-serial comparison and lexical path comparison. It still copies map keys
into its own slice with capacity reserved for every key, and returns a nonnil
empty slice for empty input. Neither the input map nor its evidence changes.

`summaryPathTooDeep` shares the publication depth check across named slots,
result contents and requirement candidates. For nonempty serialized paths,
the step count is one greater than the separator count. Empty paths remain
inside the fixed positive bound; empty components still count as before.
Counting separators avoids materializing a split that those checks never use.
Queries that need actual components retain `SplitAccessPath`. Path limits,
slot limits, proof budgets, truncation policy and root naming remain unchanged.

Three-sample medians use the same warmed RAM cache, two workers and reduced
priority. Benchmark setup, including labels, is outside the timed sections:

| Primitive | Before | Candidate | Allocations before/after |
| --- | ---: | ---: | ---: |
| Sort zero slots | 34.79 ns | 12.34 ns | 1 / 0 |
| Sort one slot | 85.05 ns | 61.97 ns | 2 / 1 |
| Sort eight slots | 530.1 ns | 211.9 ns | 4 / 1 |
| Sort thirty-two slots | 2059 ns | 1040 ns | 4 / 1 |
| Check root depth | 18.87 ns | 18.60 ns | 0 / 0 |
| Check depth at limit | 46.62 ns | 17.68 ns | 1 / 0 |
| Reject depth beyond limit | 62.26 ns | 16.30 ns | 1 / 0 |

The depth checks remove 48 bytes at the limit and 64 bytes beyond it. Sorting
eight or thirty-two slots removes 96 bytes per call. These measurements are
primitive results, not a whole-program speedup claim. Regression tests preserve
serial order, lexical rather than numeric path order, unchanged source evidence,
empty output ownership, exact accepted/rejected depths, placeholder prefixes,
named roots, unknown roots and empty path components. Existing projection
fixtures cover state/history/escape publication at the boundary.

The first completion gate catches `fmt.Sprint` in benchmark labels; switching
them to `strconv.Itoa` changes no timed code. Raw benchmark and gate receipts
remain in the RAM workspace's `projection/`. The overall audit remains open:
these two measured candidates are addressed, while other projection costs,
guard filtering, visited maps and catalog engine profiles remain to review.

After the label correction, all eight canonical `make verify` gates pass.
Coverage passes at 92.5% using the target's commands with its output paths
redirected into RAM; the README badge remains accurate. Self-analysis with all
checks stays clean. Small benchmark, final gate and coverage-summary receipts
are retained in `.build/perf-projection-20261006/`; caches and the large coverage
profile remain in RAM. No local race test or full precision replay was run.
Hosted CI for parent `b88b8be1` is fully green; new-commit hosted results remain
separate. The broader completion audit still needs representative catalog
profiles beyond the lock/process evidence and a larger controlled target pair.

## Larger-target audit finds guard handling worth pursuing

Three matched Caddy pairs compare production bodies at `20973ecb` with
`18c1e035`. The baseline is rebuilt from the current checkout using an overlay
of the five changed production files; both executables have identical actual
CLI Run timing wrappers. The wrapper's production CLI body is checked against
the current source. Both use the same warmed RAM build cache, two workers,
reduced priority, pinned Caddy tree and distinct inactive candidate flags to
force fresh analysis.

| Pair | Before | Candidate |
| --- | ---: | ---: |
| 1 | 94.605 s | 95.361 s |
| 2 | 96.303 s | 95.104 s |
| 3 | 99.547 s | 102.611 s |

Medians are 96.303 and 95.361 seconds; the candidate is faster in only one
pair, so the recent heap primitives establish no convincing end-to-end
speedup. All six full diagnostic JSONs match the known two findings, with exit
3 and empty stderr. Each records 14924 Runs, 1066 for each of eight catalog
roots and six prerequisites, and only 16 KiB of physical child-process writes.
These observations do not undo earlier large improvements; the baseline
already contains them. Median summed lifecycle-fact Run times are 75.239 and
73.862 seconds. These concurrent action sums do not partition wall time.

One further serial all-check Caddy scan profiles six remaining catalog roots
on distinct ordinary dependency packages, plus lifecyclefacts on certmagic and
resultfacts on reflect. Distinct packages prevent overlapping profilers inside
one process. It preserves all diagnostics and action counts, exits 3 with
empty stderr, and writes 4 KiB physically. Raw data stays in the RAM workspace's
`catalog-audit/`; small receipts and focused summaries are retained under
`.build/perf-catalog-audit-20261006/`. Short catalog CPU intervals and sparse
allocation samples cannot prove those engines free of opportunities.

The substantial remaining evidence is certmagic lifecycle-fact inference:
its Run takes 12.888 seconds. Its process allocation snapshot is 4202.24 MiB;
focused lifecycle stacks attribute 735.60 MiB flat to guard filtering and
974.60 MiB cumulative to loaded-guard identities. These are allocations since
process startup, not an exclusive interval measurement. The CPU interval
samples all goroutines; focused lifecycle stacks include 1.510 seconds
cumulative in guard-key construction. The broader goal is still unproven.

### Bounded guard filtering without repeated slice growth

The filter now collects the normal bounded guard list in a stack buffer, then
allocates and copies exactly the retained entries. Oversized inputs retain the
original growing-slice behavior. Both paths delegate to one charged filtering
loop. Nil empty results, input ownership, guard order, values/stability, matching
and every budget charge remain unchanged; a cutoff discards partial output.
The new helper belongs beside the existing invalidation policy because it
separates allocation strategy from that one authoritative loop, rather than
introducing another evidence model.

Three-sample primitive medians on the same warmed RAM workflow are:

| Guards retained from eight | Before | Candidate | Bytes before/after | Allocations before/after |
| --- | ---: | ---: | ---: | ---: |
| Zero | 39.88 ns | 40.88 ns | 0 / 0 | 0 / 0 |
| One | 74.50 ns | 59.88 ns | 24 / 24 | 1 / 1 |
| Four | 122.70 ns | 85.77 ns | 144 / 96 | 2 / 1 |
| Eight | 211.80 ns | 113.20 ns | 336 / 192 | 3 / 1 |

Regression tests cover unchanged and selective output independence, preserved
guard fields/order, empty results, oversized lists, exact and partial local
allowances, shared-pool cutoff and charging removed entries. No identity bytes,
matching rules or path feasibility policy changes accompany this optimization.

The subsequent all-check Caddy profile preserves its complete two-finding JSON,
exit 3, empty stderr and all 14924 action records. It writes 16 KiB physically.
On lifecycle-focused stacks, guard filtering falls from 735.60 to 341.06 MiB
flat allocation, about 54% less. The process allocation snapshot falls from
4202.24 to 3441.75 MiB. These sampled, process-since-start snapshots support the
allocation reduction, not a peak-memory claim. The selected certmagic Run takes
15.273 rather than 12.888 seconds, and the complete profiled command takes
110.565 seconds. The earlier run profiled eight packages whereas this one
profiles only certmagic, and host load varies. No latency improvement is claimed.
Guard identity generation and visited-state/key work remain substantial costs.

All eight canonical local `make verify` gates pass, including all-check
self-analysis and the full test suite. Raw profiles and large files remain in
the RAM workspace's `guard-filter/`. The performance goal remains active.

Coverage passes at 92.5% with the canonical target's output paths redirected
into RAM; the README badge remains accurate. Small benchmark, gate, coverage
summary, Caddy receipt and focused profile summaries are retained in
`.build/perf-guard-filter-20261006/`. No local race test or full precision replay
was run. The parent `18c1e035` hosted workflows are green; new-commit checks
remain separate. The next guard opportunity must address immutable identity
construction without changing identity bytes or traversal/budget semantics.
