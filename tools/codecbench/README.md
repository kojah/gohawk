# Fact payload codec experiment

This separate module keeps Protobuf benchmark dependencies and generated types
out of the shipped analyzer. The schemas cover the complete result fact and
heap-summary payloads, not the complete lifecycle/concurrency publication model.
They are prototypes, not a supported wire contract.

```sh
cd tools/codecbench
GOMAXPROCS=4 go test -bench . -benchmem -count=3
```

Generated with protoc 36.2 and protoc-gen-go 1.36.12. From this directory, with
those executables on PATH:

```sh
protoc --go_out=. --go_opt=module=github.com/kojah/gohawk/tools/codecbench result.proto heap.proto
```

Every benchmark includes encoding the existing Go model, parsing, and conversion
back into that model. Protobuf uses deterministic marshaling. The fixtures are
synthetic bounded shapes, not a measured distribution of exported production
facts. Heap adapters preallocate collections. The CBOR candidate includes its
four-byte version header; the Protobuf prototype has no framing yet.

Initial results on Xenia, Go 1.27.0, `GOMAXPROCS=4`, 2026-09-24:

| Shape | JSON round-trip | CBOR round-trip | Protobuf round-trip | Payload bytes JSON / CBOR / Protobuf |
| --- | --- | --- | --- | --- |
| 2 result guarantees + relations | ~2.2 µs | ~1.6 µs | ~0.7 µs | 133 / 98 / 18 |
| 16 result guarantees + relations | ~8.4 µs | ~5.7 µs | ~3.2 µs | 641 / 448 / 144 |
| 32 heap edges + effects + requirements | ~183 µs | ~124 µs | ~67 µs | 14195 / 9742 / 2994 |

The large heap shape allocates ~41 KB/117 objects with JSON, ~35 KB/171 objects
with CBOR, and ~76 KB/955 objects with the preallocated Protobuf adapters.
Protobuf wins CPU and size here, not allocation count or bytes. These payload
microbenchmarks cannot establish whole-analyzer speedups, especially where GC
already consumes substantial CPU. No compiler or analysis budgets were reduced.

Before production adoption:

- Complete the lifecycle and concurrency schemas and test every field.
- Check integer/enum narrowing, schema versions, bounds, and missing fields.
- Explicitly represent meaningful presence; repeated fields normalize nil and
  empty collections in these prototypes. That is not an exact arbitrary-Go-value
  round-trip guarantee.
- Decide whether to maintain adapters or adopt generated publication types.
  Neither choice should force generated pointer-heavy types into proof engines.
- Test full fact-driver round-trips and end-to-end normal/race analyzer runs.
- Preserve unknown evidence: missing/incompatible data must never become an
  empty complete synchronization summary or an unconditional lifecycle proof.
