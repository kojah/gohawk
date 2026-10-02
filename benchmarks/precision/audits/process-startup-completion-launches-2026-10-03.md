# Startup wrapper completion launch selection

Bead: `gohawk-dho.23.20`. Parent: `0711d46`.

## Evidence and decision

The pinned Ferro trace queried ordinary `configureProcGroup(cmd)` completion
against a later `StdinPipe` result tuple. Actual SSA is in
`.build/goal-process-owner-relevance-ssa.log`; the relevant values are t40
(void configuration), t42 (pipe tuple) and t43 (pipe projection). Definition
order alone is not a safe filter: later results can alias existing objects.

Instead the existing startup wrapper policy accepts only
`EvidenceDeferredCompletion`. Shared completion derives its top-level reason
from the launch form, including returned-callback invocation: ordinary calls
remain called completion even if their body defers cleanup, and goroutine
launches remain started completion. `testing.Cleanup` is the exact ordinary-call
exception that the shared resolver classifies as deferred. The startup consumer
now selects only `ssa.Defer` and synchronous calls with that existing contract
before submitting owner completion requests. No new traversal, callback
resolver, lifecycle mode, name exemption or fact schema was added. Command
cleanup/transfer policy and later watcher discovery remain separate.

## Controls and validation

Actual SSA controls distinguish direct calls, helpers with nested defers,
goroutine launches, deferred helpers, deferred helpers without watchers and
`testing.Cleanup` registrations. Non-deferred instructions use no completion
allowance, while deferred cutoffs preserve unknown startup ownership.
Complete deferred supervision still requires a later watcher. A parent overlay
fails all three non-deferred zero-allowance assertions, establishing that the
old query abandoned irrelevant work and treated it as uncertain registration.
Controls live in `startup_queries_test.go`; existing registered-owner and
supervisor fixtures remain covered by the full process package test.

Focused process tests and `make verify VERIFY_TIMINGS=1` pass: generation,
formatter, modules, vet, dead code, lint, ordinary tests and self-dogfood.
Ordinary tests took 55 seconds and dogfood 23 seconds. The canonical dogfood
binary has the same SHA256 as the frozen comparison binary. No local race
run or full precision replay was performed.

Full all-check fixture payloads are identical: producer 22, process 41, with
no added or removed diagnostics. Receipts:
`.build/goal-process-owner-relevance-fixtures/comparison.json`.
Frozen parent SHA256:
`4bd77c458e07fffa0b34fa9564c6b03e5fc2d0dc01a26fd8ef61008aa9a506c0`.
Frozen current `.build/goal-process-owner-relevance-reviewed` SHA256:
`cb52d49e7685fb8fe7c5b9905063c369d61d2f78a81d7e28a0a7032d20b80db5`.

The clean pinned Ferro scope `./mcp` at
`d025ca1a3c6e0c6a83ed7c93147e36f39a1e6cb4`, every check enabled, CGO disabled,
readonly modules and GOWORK off, exits 0 with empty stderr. Its complete
payload is identical: one unrelated registry goroutine diagnostic. Trace size
falls from 317396 to 81654 bytes; no runtime speedup claim is made. The original
process site still ends unknown with `command-use-budget-exhausted`, so its
silence receives no FP credit. Constructor/receiver-state guarantees remain
unresolved. Receipt and trace: `.build/goal-process-owner-relevance-ferro/`.
Seven recorded production sites plus Rune remain open. No full precision
replay is run for this bounded correction.
