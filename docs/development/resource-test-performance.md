# Resource test performance investigation

Status: initial measurements, not a completed performance fix. Tracked by
`gohawk-rey`; codec alternatives are tracked by `gohawk-a1h`.

## Controlled normal-run comparison

On 2026-09-24, compare these immutable revisions on Xenia:

- Old: `02e511767da3dd0da49ee23036a1bd606a672a74`.
- Current: `8773e9eeb5631437ce81b07a3cad00debccdae43`.

Both use Go 1.27.0 linux/amd64, x/tools v0.49.0, `GOMAXPROCS=4`, and
`resourcelifetime.TestConfiguration`. Its configuration fixture is unchanged
between revisions. Compile each test binary before measurement, then alternate
old/current runs three times. No other heavy validation runs alongside them.
Build/export caches are warm and shared; test results are not cached. These are
not cold-cache measurements or timings of the complete resource test suite.

| Revision | Wall seconds, samples 1/2/3 | Peak RSS KiB, samples 1/2/3 |
| --- | --- | --- |
| Old | 20.65 / 19.08 / 19.29 | 2192892 / 2181888 / 2183508 |
| Current | 40.16 / 40.27 / 42.08 | 2460500 / 2456772 / 2451464 |

All six runs pass. Median wall time increases from 19.29s to 40.27s, about
2.09 times. Median peak RSS increases about 12.5%. This reproduces a normal-run
regression; race instrumentation alone cannot explain it. It does not identify
the introducing commit or establish that every test regressed by this factor.

Build from each checkout root:

```sh
GOMAXPROCS=4 go test -c -o /absolute/output/resource.test \
  ./internal/analyzers/resources/resourcelifetime
```

Run from that checkout's `internal/analyzers/resources/resourcelifetime`:

```sh
/usr/bin/time -v -o /absolute/output/sample.time \
  env GOMAXPROCS=4 GOROOT=/home/james/.proto/tools/go/1.27.0 \
  /absolute/output/resource.test \
  -test.run '^TestConfiguration$' -test.count=1 -test.v
```

The explicit GOROOT makes direct test-binary execution agree with the selected
Go command; otherwise this host's inherited GOROOT fails analysistest's toolchain
check. Test compilation is excluded from the table. Logs and profiles are retained
locally under `.build/perf-comparison-2026-09-24/`; temporary source checkouts are
removed after measurement.

## Current-revision CPU profile

A separate current-revision run, with `-test.cpuprofile` and
`-test.memprofile`, passes in 41.17s. It contains 117.83 CPU-seconds of samples;
CPU time exceeds wall time because work runs concurrently.

| Cumulative stack | CPU seconds | Share of samples |
| --- | --- | --- |
| checker `codeFact` | 71.89 | 61.01% |
| gob `sendTypeDescriptor` | 26.55 | 22.53% |
| gob `decodeTypeSequence` | 25.79 | 21.89% |
| `heapmodel.ProjectHeap` | 13.32 | 11.30% |

These stacks overlap: do not add the percentages. Separately filtering stacks
through `factcodec`, `GobEncode`, or `GobDecode` accounts for 13.43 CPU-seconds
(11.40% of all samples). Replacing JSON alone therefore does not address the
largest observed serialization overhead.

In x/tools v0.49.0, `checker.codeFact` encodes each inherited fact into two fresh
gob streams, compares the bytes for determinism, then decodes another instance.
This is deliberate validation, not work to bypass. A modular production driver
can amortize descriptors over a longer stream, so this profile must not be
presented as production-driver performance.

In this Go toolchain, `encoding/gob.Encoder.sendActualType` recursively visits
exported fields even for a type with a custom GobEncoder. The existing JSON
payload avoids part of gob's work but does not hide the nested summary schema
from this traversal. This is a source-confirmed mechanism and a candidate for
measurement, not yet a demonstrated fix.

## Next experiments

1. Compare the existing fact with an opaque publication wrapper that contains
   the same summary behind an unexported field. Measure the full two-encode,
   one-decode round-trip, not just JSON throughput. Preserve deterministic
   bytes, complete values, and malformed-input handling.
2. Measure end-to-end improvement before selecting a wrapper or codec change.
   Keep the semantic summary representation independent of its publication.
3. Profile the old revision and narrow the regression interval. Attribute
   loading, SSA, summary inference, serialization and harness overhead without
   summing overlapping action durations.
4. Add isolated cold-cache and repeated race comparisons without clearing shared
   caches. Existing race timings overlapped validation and are not regression
   baselines.

The separate immutable resource race suite at `a55f399` passes in 983.071s
with `GOMAXPROCS=4`, `-race -p=1 -timeout=30m -count=1`. It establishes a valid
race result, not acceptable runtime or a performance improvement. Correcting
the recursion assertion to time its analyzer action instead of its entire test
harness similarly fixes attribution, not analysis speed.
