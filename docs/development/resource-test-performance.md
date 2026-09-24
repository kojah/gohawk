# Resource test performance investigation

Status: baseline investigation and binary publication measurements. Broader
performance work is tracked by `gohawk-rey`; codec work by `gohawk-a1h`.

## Controlled normal-run comparison

On 2026-09-24, compare these immutable revisions on Xenia:

- Old: `02e511767da3dd0da49ee23036a1bd606a672a74`.
- Current: `8773e9eeb5631437ce81b07a3cad00debccdae43`.

Both use Go 1.27.0 linux/amd64, x/tools v0.49.0, `GOMAXPROCS=4`, and
the resource analyzer's configuration test. Its fixture is unchanged
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
from this traversal. This source-confirmed mechanism motivated the publication
change and measurements below.

## Binary publication change

The selected implementation separates semantic summaries from domain-owned
opaque publication envelopes, caches immutable encoded payloads, and uses
`github.com/fxamacker/cbor/v2` v2.9.0 with deterministic encoding. No proof rules,
fixtures, or analysis budgets change. The private wire format is versioned;
there is no JSON fallback. See [fact model](fact-model.md#binary-publication).

The sparse lifecycle-fact microbenchmark includes the checker's two fresh gob
encodes and one decode. Three samples on Xenia give:

| Publication | Median time | Allocated bytes | Allocations | Gob stream bytes |
| --- | --- | --- | --- | --- |
| Exposed summary with JSON payload | 102.6 µs | ~48226 | 467 | 2094 |
| Opaque cached CBOR envelope | 25.0 µs | 13560 | 200 | 623 |

Run `GOMAXPROCS=4 go test ./internal/factcodec -run '^$' -bench . -benchmem -count=3`
to reproduce the fixture comparison. This is not a representative distribution
of every production fact and does not establish a whole-analyzer speedup.
The combined change includes removing gob descriptor work and caching, not just
replacing JSON. The separate Protobuf comparison and its allocation/presence
tradeoffs are recorded in [the benchmark module](../../tools/codecbench/README.md).

### End-to-end normal-run repeats

Repeat the same configuration fixture with the retained baseline binary built
from `8773e9e` and the CBOR candidate binary. Both run from the current package
directory: the fixture and test source are unchanged from that baseline. Use
the same toolchain, flags, warm build/export cache, and alternating three-pair
method above, with no competing heavy validation. All six runs pass.

| Publication | Wall seconds, samples 1/2/3 | Peak RSS KiB, samples 1/2/3 |
| --- | --- | --- |
| Baseline JSON | 44.60 / 40.71 / 41.04 | 2463784 / 2453668 / 2464240 |
| Opaque cached CBOR | 23.13 / 21.10 / 21.38 | 2551760 / 2535380 / 2519288 |

Median wall time falls from 41.04s to 21.38s, about 48%. Median peak RSS rises
about 2.9%; retaining encoded payloads is not free. This establishes an
improvement for this normal analysistest workload, not the production driver,
the full resource suite, or a race-instrumented speedup. The candidate binary
predates the final decoder-only rejection of the CBOR undefined token; valid
fact encoding, decoding, and all analysis code are the measured implementation.

Ordinary repository verification passes. A local full race run was stopped
before the resource suite completed when the CI-only race policy was clarified;
it is not a passing full-suite receipt. The targeted CI race gate now includes
the shared fact encoding cache. Do not rerun race tests locally for this
investigation; measure ordinary runs here and record race results from CI.

### Follow-up profile and envelope layout

A separate normal CPU profile of the CBOR candidate passes in 23.01s and
contains 64.80 CPU-seconds of samples. Checker fact round-trips account for
29.81 CPU-seconds (46.0%), down from 71.89 in the pre-change profile. Heap
projection is 13.48 CPU-seconds (20.8%), close to the earlier 13.32; optimizing
publication did not reduce the underlying heap inference work. These are
overlapping cumulative stacks, not additive stages. The old `02e5117` profile
also passes (21.29s); its fact round-trips account for 43.36 of 64.67 sampled
CPU-seconds. The richer current model is now near the older normal runtime,
but these profiles do not bisect the original regression to one commit.

Making the envelope's embedded field private removes its own additional gob
descriptor without changing its promoted methods or payload. A fresh three-run
microbenchmark records 12616 B/190 allocations per round-trip versus
13568 B/200 allocations for the exported embedding. Median times are 19.6 µs
versus 21.6 µs, with visible timing noise. One ordinary end-to-end private-layout
run passes in 21.42s with 2425676 KiB peak RSS. That single sample is not proof
of an additional end-to-end speed or memory improvement; the allocation and
descriptor reduction are the reason to keep the simpler wire shape.

## Remaining experiments

1. Profile the old revision and narrow the regression interval. Attribute
   loading, SSA, summary inference, serialization and harness overhead without
   summing overlapping action durations.
2. Add isolated cold-cache measurements without clearing shared caches. Any
   further race comparisons belong in CI. Existing race timings overlapped
   validation and are not regression baselines.

The separate immutable resource race suite at `a55f399` passes in 983.071s
with `GOMAXPROCS=4`, `-race -p=1 -timeout=30m -count=1`. It establishes a valid
race result, not acceptable runtime or a performance improvement. Correcting
the recursion assertion to time its analyzer action instead of its entire test
harness similarly fixes attribution, not analysis speed.
