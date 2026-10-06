# Performance work on a shared host

Fresh Go analysis publishes dependency facts and writes build-cache entries.
Compiler workspaces and concurrent validation add more temporary output. These
writes can delay other jobs that depend on filesystem-journal commits, even
when memory and CPU look healthy.

## Keep disposable writes in RAM

On Linux, use a private tmpfs workspace with the repository wrapper:

```sh
scripts/with-ram-go.sh /dev/shm/gohawk-perf make verify
scripts/with-ram-go.sh /dev/shm/gohawk-perf make benchmark BENCHMARK_ARGS='--only self --runs 3'
```

The same workspace retains its Go cache between commands. The wrapper sets
`GOCACHE`, `GOTMPDIR`, and `TMPDIR` there, limits Go concurrency to two, and
sets `VERIFY_JOBS=1`. It runs at reduced CPU priority and idle I/O priority
when `ionice` is available. A nonblocking workspace lock prevents overlapping
commands using that workspace. stdout and the command's exit status are
preserved; a disk-backed workspace is rejected.

Set `GOHAWK_GO_PROCS` to choose a different Go concurrency limit. These
settings are part of the measurement environment: compare binaries using the
same workspace settings, cache state, and parallelism. A RAM-backed run with
two workers is not directly comparable with an earlier disk-backed run using
the host defaults.

Put timing files, CPU/allocation profiles, and verbose temporary output in
the RAM workspace too. Copy only the useful final receipts and summaries to
the repository after a job ends. Module downloads and explicit result paths
retain their configured locations; reuse already downloaded dependencies.
Run verification, coverage, and benchmarks sequentially. Request a fresh
analysis cache only when the experiment needs it; retain compiled artifacts.

The workspace consumes RAM and shares the tmpfs filesystem's capacity. Check
available RAM and tmpfs space before a large run. Remove only the workspace
you created when it is no longer useful; do not clear shared Go caches or
another session's temporary directories. Retain the workspace while iterating
so the next command can reuse its cache. RAM contents disappear on reboot.

## Measure the writes

On Linux, sample `/proc/PID/io` across the relevant process tree to locate
active readers and writers. `/proc/pressure/io` shows system-wide stalls but
does not attribute them to a particular job. Historical cache size is likewise
not proof of who is currently writing.

For a command whose descendants wait for their children, record the change in
`resource.getrusage(resource.RUSAGE_CHILDREN)` around that command. Linux's
`ru_inblock` and `ru_oublock` count 512-byte filesystem I/O blocks. This
separates physical I/O from bytes written to tmpfs or already cached reads.
Record diagnostics separately so resource scheduling changes cannot conceal a
behavior change.

The October 6 investigation uses a private workspace at
`/dev/shm/gohawk-perf-01a0f86c`. Its first self-analysis, including population
of a fresh RAM Go cache, performs about 36 KiB of child-process physical writes
and 0.72 MiB of physical reads. The first Caddy analysis performs about 192 KiB
of physical writes and 129.6 MiB of reads while populating its RAM cache.
Complete diagnostics match the earlier runs, including Caddy's two findings.
The workspace and its detailed records are temporary; see the maintained
[performance investigation](analysis-performance-2026-10-06.md) for receipts
and measurement limits.
