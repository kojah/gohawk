# Local command pipe IO follow-up — 2026-10-02

Beads: `gohawk-dho.4.4`. Parent: `707913f`. The
[eleven-site reassessment](remaining-eleven-reassessment-2026-10-02.md)
identified rev-dep's local pipe operations as a bounded correction to the
existing unused-command ownership decision. Historical labels remain intact;
the [follow-up ledger](process-local-pipes-followup-2026-10-02.tsv) records the
affected site and two production true-positive controls.

## Evidence boundary

The existing `handsValueOn` classifier now excludes synchronous Read/Write/Close
calls on exact standard IO pipe results of `exec.Cmd`. Operand provenance also
stops at those operations' returned errors/counts: returning Close's error does
not hand the child process to another owner. Both consumers use the same private
`commandPipeOperation` contract composed from shared call-result/receiver and
symbol identity queries. No alternate report decision, traversal, fact schema,
completion guarantee, project exemption or budget allowance is introduced.

Returning a pipe, handing it to other code, or launching its operations with
Go/Defer retains ordinary ownership evidence. The initial broad pipe-result
prototype hid `orphanedPipe`; that version was rejected. The final correction
preserves that diagnostic as well as partial Wait, Kill, and unrelated methods
named StdinPipe. Fixtures cover local input/output/error pipe operations beside
those diagnostic forms. Final trace assertions require one unused-command
unknown decision without budget exhaustion for each accepted local IO fixture
and the existing browser launch.

This leaves genuine IO-only process leaks undetected, just as the existing
unused-command boundary leaves intentionally detached launches alone. Neither
pipe Close nor detachment proves Wait/Release; no cleanup summary is published.
The analyzer remains experimental. The design note and public detection page
state the changed boundary.

## Validation

Focused analyzer tests pass in `.build/goal-process-pipes-focused-final.log`.
Two ignored overlays independently remove the local-call classification and
the IO-result provenance stop. Both fail accepted fixture and final-reason
assertions, rather than compilation. Receipts
`.build/goal-process-local-pipes/{local-call,io-result}-counterfactual-final.log`.
The original broad prototype failure is retained separately.

The first canonical gate passed seven targets but hit the instruction walk's
cognitive-complexity limit. Moving the call distinction into the existing
instruction classifier keeps the walk's complexity unchanged. Final
`make verify VERIFY_TIMINGS=1` passes all eight targets: generation, module
verification, formatting, vet, deadcode, lint, self-dogfood and ordinary tests
including architecture checks. Receipt
`.build/goal-process-local-pipes-verify-final.log`. No full precision replay or
local race test ran.

Immutable final binary `.build/goal-process-local-pipes-reviewed-final`, SHA-256
`38c27d44a3ac00cbea721f53818cb64d23ac4db74c0a39b3cf7256054c1b613e`.
Parent binary `.build/goal-cache-removal-reviewed`, SHA-256
`c30d2d011ae4e9d6ac45f4d91a566aa81fbf1c768c8d19473f612385ac5cb898`.
Pins, scopes and exact position/check keys are in the ledger. All six all-check
vet scans succeed with empty stderr; modules are readonly, CGO/workspaces
disabled, timeout 180 seconds and concurrency at most two. Metadata and JSON
receipts live under `.build/goal-process-local-pipes-final/`.

Rev-dep's finding disappears (1,138-byte JSON becomes `{}`). Its final trace is
`unused-command-ownership-unknown` with no budget-exhausted events. Actual
Dispatch SSA and the trace are retained in that receipt directory. Watchdog's
early error return and diff's pipe-backed goroutines retain their reviewed
missing-wait findings and byte-identical 1,205/1,156-byte JSON.

This credits one structural correction: ten production sites plus Rune remain
semantically unresolved. Seven of those sites were visible in the pre-fix
snapshot; three declined with budget evidence. The other six repository scopes
were not rerun for this analyzer-local change, and their earlier receipts are
not a new final-binary census. Broader architecture and fact-consumer review
remain active.
