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
