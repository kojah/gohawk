# Program-entry process follow-up

Beads `gohawk-dho.4.1` narrows the existing experimental process wait check at
its authoritative return decision. Four reviewed coder/acp-go-sdk false positives
are present in the parent and absent in corrected successful package scans.
The reviewed of-watchdog process leak remains byte-identical. This leaves **11
unresolved production FP locations** in the queue previously at 15. Original
batch verdicts and precision totals remain historical evidence.

## Evidence and coverage tradeoff

At coder pin `0845a3bb9eddda5bfc22a94dd3598c90cb842451`, the four executable
entries start children outside cycles. Actual agent SSA is retained in
`.build/goal-process-entry-agent-ssa.log`: CommandContext produces the command,
Start acquires the wait obligation, and the normal path calls Process.Kill
before returning. Kill is not Wait, and parent exit does not guarantee child
termination. An uncovered return in this bounded entry shape cannot distinguish
intentional program-lifetime ownership from missing reaping. The final decision
is therefore `program-lifetime-ownership-unknown`, not cleanup proven.

The existing `ssaflow.RunsOnceInProgramEntry` proves declaration identity,
non-cyclic execution and absence of package references to main. The same proof
already supports the cancellation entry boundary. It now also examines the
synthetic package initializer, which contains function aliases and callback
tables that declared-source enumeration misses. No new resolver, flow engine,
callee fact schema, project exemption or tier change is introduced.

Exact wait/transfer results retain precedence. Repeated starts, referenced
entries, methods, closures, reusable helpers and another package's main retain
ordinary obligations. A real missing wait in an otherwise qualifying entry can
be missed; the policy explicitly accepts that coverage loss, including resources
retained until exit and unobserved child results. It exports no callee guarantee.
The fixtures pair one-time accepted starts with repeated and initializer-aliased
entry diagnostics. Nine actual-SSA decision scopes also preserve exact settlement;
four shared-helper scopes distinguish entry aliases/tables from unrelated aliases.

Disabling only the entry decision through a Go source overlay fails both
one-time decision cases and the accepted analyzer fixture. Disabling only the
initializer reference check fails alias/table helper controls, the referenced
entry decision and the corresponding diagnostic fixture. Receipts:
`.build/goal-process-entry-disabled.log` and
`.build/goal-process-init-disabled.log`. These overlays do not edit the checkout.

## Scoped replay and provenance

Parent binary `.build/goal-ambiguous-cleanup-current` implements pushed
`7ceaa9a`, SHA-256
`3014b2da4d418695484913ed56aa9b819bc9c028274b707f5b1317ce32c2d55b`.
Corrected `.build/goal-process-entry-current` implements that parent plus this
source change, SHA-256
`06911bef0c49a369db8e386ac1039816d42cdc1685cf42544d4f33f77f9c4384`.
Neither executable was replaced during scans.

Both use `-enable-checks=processownership/missing-wait -json`,
`CGO_ENABLED=0`, `GOFLAGS=-mod=readonly`, and `GOWORK=off`. Coder scopes are
`./example/agent ./example/claude-code ./example/client ./example/gemini`;
of-watchdog is `./executor` at pin
`d0698f1e22347eb41682563f5ed4653f3dc4ce2e`. Candidate tests, generators and
applications were not executed. All four scans have empty stderr.
The [per-site ledger](program-entry-process-followup-2026-10-02.tsv) records
original diagnostic keys; SSA trace call positions differ from diagnostic starts.

Coder parent exits 3 with all four reports; corrected exits 0 with empty JSON.
Their trace receipts `.build/goal-process-entry-coder-check-{parent,current}.trace.jsonl`
show each final decision changing from `unowned-return`/rejected to
`program-lifetime-ownership-unknown`/unknown. Of-watchdog exits 3 in both scans,
retaining the reviewed successful-Start error return bypassing its later Wait.

Initial scans used `-enable=processownership`, which selects the analyzer but
leaves this experimental check disabled under the core ceiling. Those empty
outputs (`goal-process-entry-{coder,watchdog,znkr}-parent` and coder-current)
provide no precision evidence and are superseded by the explicit-check scans.
No absence or retained TP is credited from them.

Focused tests pass for process entry, shared entry identity, reason codes and
cancellation result guards. The canonical gate passes (`make verify
VERIFY_TIMINGS=1`, `.build/goal-process-entry-verify.log`): ordinary tests 56s,
local dogfood 29s, vet, lint, deadcode, formatter, module and generated checks.
No local race or full precision-regression run is part of this iteration.

The shared entry-proof correction also has a sibling-analyzer control: Openase
pin `e530faf137e764337d5beaaf68af3be159eb17aa`, `./internal/orchestrator`,
`-enable-all -json` under the same environment. Parent and corrected scans both
exit 3 with empty stderr and byte-identical JSON, retaining the two reviewed
cancellation TPs listed in the ledger. Receipts are
`.build/goal-process-entry-openase-{parent,current}.json`. This is a scoped
control, not a latest full-repository precision claim.
