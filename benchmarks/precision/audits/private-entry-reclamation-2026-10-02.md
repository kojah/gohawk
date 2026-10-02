# Resource reclamation through private entry calls

Beads `gohawk-dho.4.6`.

Boxesandglue's helper package has a unique synchronous chain from the language
entry: `main -> dothings -> createPatterns`. Actual SSA confirms that neither
call nor the file acquisition is in a CFG cycle. The output file's error
returns do not close it, while success closes it. The existing reclaim-only
program-exit policy covered only acquisitions directly in main.

## Implementation boundary

One shared declaration-resolved private-use collector replaces lockorder's
operand census and serves the new entry query. It retains the 32-call cap,
unknown/escaped uses, initialization and caller-supplied body scope. Operand and
instruction visits now consume the supplied allowance; interrupted discovery
publishes no caller prefix. Lockorder retains its private exclusivity and
conditional-release policies.

The resource decision uses `RunsOnceThroughPrivateEntryCallsWithin`. Each
private declaration needs one synchronous caller and no escaping uses. Every
call and the acquisition must be acyclic, and the chain must reach the exact
language entry without references that could run main again. Generic
declarations and chains beyond sixteen frames decline. Package metadata
enumeration retains its existing independent cost; the actual body, operand
and CFG searches use the resource candidate's existing allowance. A cutoff
returns an unknown resource proof. Other analyzers retain their existing
direct-entry query and policy.

The resource-family gate remains unchanged: descriptors, response bodies and
nontransaction SQL resources qualify; compressor flushing, transactions and
inferred owners do not. No helper publishes an unconditional cleanup fact.
This is a process-local execution-count boundary, not caller error propagation
or a guess that arbitrary callers exit.

## Evidence and limits

Compiled SSA controls cover fifteen private-chain forms, both sides of the
sixteen-frame boundary, budget cutoffs and fresh-query recovery. Existing
lock inventory, exclusivity and return-contract tests exercise the shared
collector. Two counterfactuals fail assertions: ignoring escapes admits
callbacks/Go/Defer/aliases/reentry, and allowing multiple calls admits repetition.
The `privateentry` resource fixture accepts the minimized file error path and
retains repeated/looped/escaped acquisitions, compressor flushing and transaction
cleanup. Its parent binary reports the accepted acquisition as an extra finding.

The ignored prototype is superseded. Initial canonical validation passed every
target except lint; complexity and test string construction were corrected.
Final scan comparisons succeed with empty stderr across ten receipts. Parent
outputs from the completed first comparison are reused; five current outputs
were refreshed against the final immutable binary. The
[ledger](private-entry-reclamation-2026-10-02.tsv) records exits, pins, hashes and
individual receipt paths. Existing lock/resource fixtures retain all 426
reports byte-for-byte; pinned goiardi datastore/indexer retains two lock reports.
The new entry fixture goes from six reports to five, dropping only its accepted
file acquisition. Clean pinned boxesandglue `./helper` goes from two reports to
one, dropping only `pattern.go:79:14` while retaining its other finding.

Final binary SHA-256:
`0af0e315c50a7a5cdcef1286008e9315c459afb808e4e6a3aac6b86407ec43a2`.
Receipts: `.build/goal-private-entry-focused.log`,
`.build/goal-private-entry-controls-final.log`,
`.build/goal-private-entry-mutants-final/results.json`,
`.build/goal-private-entry-final/scans.json`,
`.build/goal-private-entry-verify-final.log` and
`.build/goal-private-entry-architecture-final.log`.
Final `make verify VERIFY_TIMINGS=1` passes all eight canonical targets,
including ordinary tests and self-dogfood. Final architecture validation passes.
No full precision-regression replay or local race run is used.

Graph tools were unavailable. Exact source, declaration/type identity and
compiled SSA supply the scoped evidence. This does not certify every remaining
FP family or the overall architecture. The historical eleven-site ledger
remains a snapshot. The final production scan credits one correction, leaving
nine recorded production FP sites plus Rune. Budget-driven silence in other
families remains unresolved.
