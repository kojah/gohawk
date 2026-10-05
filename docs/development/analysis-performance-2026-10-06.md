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
