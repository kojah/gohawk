# Complete review of the remaining 36 false positives

This pass reviews every location left by the first bounded follow-up, rather
than selecting another small subset. The [36-entry ledger](batch-50-remaining.tsv)
pins every site and records source evidence separately from correction status.
Original verdicts in `batch-50-findings.tsv` are not rewritten. A reviewed gap
is not a verified correction.

**Result: all 36 reviewed, 11 corrected through bounded uncertainty, 25 still
unresolved.** Including earlier corrections, 30 of the original 55 FP locations
now have correction evidence. The 11 are Sake's four captures, Metatube's two
captures, Neffos's four lock findings, and Goshs's retained map owner.
Implementation commit: `71e65c0`.

These changes do not prove the programs safe. They stop claiming a defect
where the available evidence is insufficient. Loaded guards that really change,
worker guards that do not actually partition writers, multi-slot semaphore
regions, and retained owners that are never closed can consequently be missed.
Direct shared writes, unrelated or already-ended channel regions, unconditional
lock leaks, and proven read-only owner observers remain checked.

## Evidence families and next steps

| Family | Sites | Assessment and boundary |
| --- | ---: | --- |
| Repeated loaded lock guards | 4 | Neffos takes Message by value and repeats the same flag around matching operations. Distinct loads do not prove contradictory flag values. Narrow uncertain acquisition guards; do not assert address equality proves unchanged contents. |
| Semaphore-protected captures | 4 | Sake's Step mode restricts the semaphore to one worker. Consider only a same-channel send/receive region enclosing the write, not arbitrary channel activity. Such a region supports uncertainty, not a proof of capacity or serialization. |
| Worker-specific writes | 2 | Metatube passes distinct iteration indices to workers whose guards select different destinations. Narrow where the write depends on a worker-varying argument; do not build a loop-cardinality solver. |
| Transitive publication ordering | 1 | Benthos publishes via a mutex and receives a uniquely tagged message before its next launch. A nested receive alone does not establish that relation. Retain as a protocol-model gap. |
| Caller transaction cleanup | 3 | PostgreSQL callers defer rollback of the exact supplied transaction, including the rows' early-error paths. A borrowed Tx alone proves no prompt cleanup: long-lived transactions can still leak rows. Requires bounded caller-to-parent-cleanup evidence, not a borrowed-Tx exemption. |
| Intentionally failed acquisitions | 3 | Two golang/exp examples assume absent relative files; oss-rebuild uses read-only SQLite in a fresh empty directory. Filesystem assumptions and API failure contracts are missing. Do not exempt test files or path names. |
| Empty HTTP responses | 2 | Goravel and sigstore produce empty bodies in these tests. Proving that through handlers, transport, and client settings is beyond the current evidence. NoBody alone is insufficient when a client timeout adds cleanup state. |
| Disposition/ReadOnly guard correlation | 3 | Goshs rejects creation dispositions before acquisition in ReadOnly mode. The later rejection cannot follow these acquisitions. Needs stable guard correlation; other OpenFile paths can genuinely leak and must not be exempted. |
| Aggregate map retention | 1 | Goshs retains the file through an SMB handle and sync.Map, then closes through Range. Investigate opaque retention of the aggregate, not proof that any Map.Store closes anything. |
| Branch-merged owner cleanup | 1 | Opskat closes the branch-selected entry on handle-ID exhaustion and otherwise retains it. Requires field/nil-guard correlation; mere construction of an owner must not suppress a leak. |
| Raw-descriptor aliases | 4 | Troubleshoot passes File.Fd through a netns handle that later calls unix.Close. The neighboring line208 has the same conversion shape but genuinely lacks cleanup. Blanket Fd or mock.Return acceptance is unsound; needs alias plus lifecycle evidence. |
| Retained process owners | 4 | Fence has two field-held commands and one captured collection; kluctl registers conditional owner cleanup before filling its command field. Cleanup and success transfer are real, but the current Start-local proof misses their connection. Missing modeling, not fundamentally unknowable intent. |
| Intentional relaunch | 1 | Opskat's helper waits for its parent to exit before relaunching it. Requiring the parent to wait contradicts that lifecycle. Recommend separately reassessing the detached audit; Setsid is not evidence of reaping, and no check is retired here. |
| Cross-goroutine cancellation feasibility | 1 | Playground can skip cancel only after a worker returns; that worker returns only after the same context is canceled. A generic receive cannot prove this. Requires causal reachability evidence, not a context-use exemption. |
| Selected timeout edge | 1 | Tonutils cancels on every exit; its only uncanceled continuation selected the exact timeout's Done. SSA jumps directly from that selected edge to the loop header. Instruction-only settlement cannot express it; accepting the whole select would hide other-arm leaks. |
| Pre-registered returned worker owner | 1 | Panel test cleanup captures the returned control before its cancel/done fields are replaced; stop then pause joins the current worker. Need returned-owner retention and later field contents tied to that registration. |

The process-owner cases share a promising evidence connection, but a stored
handle or an arbitrary Close method is not enough. The cancellation cases
need different control-flow evidence. Neither warrants a general heap engine
or a new collection of framework-name exceptions.

All candidate work is static analysis and source review. No candidate tests,
generators, applications, or benchmarks are run. No checks are retired,
demoted, or disabled, and no GitHub comments are posted.

## Rejected shortcuts

An exploratory Opskat nil-guard rule removed its warning, but owner containment
could not establish that the particular field being closed held this resource.
That rule was removed rather than shipping a suppression for the wrong field.
Similarly, aggregate retention is restricted to separate struct-pointer owners:
applying it to arbitrary slices swallowed the genuine raw-descriptor leak at
Troubleshoot line208. The broader version was rejected.

## Replay scope

Round 54 passes 17 labels across three scannable repositories: seven FP
suppressions and ten TP controls. All checks and test source are enabled.
Its finding baseline retains the original scan, so eight expected removals
(the seven new corrections and Sake's previously corrected lock finding) are
reported as baseline drift; no new findings appeared.

Neffos is verified separately at its pinned root package: its four false
positives are absent and the `conn_namespace.go:145` contradictory-order
control remains. The whole-repository runner cannot validate this repository
because an unrelated stress-test example module lacks required go.sum entries.
It is therefore not included in the passing round-54 cohort. This scoped replay
does not claim that its example modules scan successfully.

The original frozen batch scanner is unchanged. Corrected executable hashes
identify local validation artifacts, not a release or clean-revision stamp.

The final corrected binary has SHA-256
`d868b45792da6b68c22f294a0e9caaa6a23879a44bdc9a8380ed0b2273f0d538`.
Full `make verify` passes in an isolated source snapshot, including generation,
module verification, formatting, vet, lint/dead-code, self-analysis, all gohawk
tests, and the trace-component race tier. Focused raw-descriptor replay retains
Troubleshoot's genuine line208 leak alongside its four unresolved false positives.
The ledger validates as 36 unique original FP locations with matching pins:
11 verified uncertainty narrowings and 25 unresolved reviews.
The previous round-53 cohort also passes all 25 labels with scannability
required, preserving its 14 FP suppressions and 11 TP controls.
