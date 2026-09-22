# Lock follow-up within the remaining 207 findings

The 23 unresolved lock findings from `followup-78-locks.tsv` were replayed with
the canonical all-check CLI. **Nine are now absent; fourteen remain active.**
Both earlier surf corrections remain absent and all four runnable genuine-bug
controls remain reported. `followup207-locks.tsv` retains all 29 sites and their
baseline/current receipts; a successful load is required before counting absence.

## Reassessment and changes

The retained checks still have useful bounded proof domains. These changes
extend exact local evidence or recognize positive uncertainty at existing
decision points; no check was retired, disabled, demoted, or given a project
or function-name exception.

| Evidence family | Newly absent | Decision |
| --- | ---: | --- |
| Local Boolean lock state across blocks | 2 bazel-remote | Carry at most four selected Boolean phi constants; refresh all incoming values together and forget unknown inputs. |
| Literal phase across retries | 1 uhaha | The same four-slot environment retains exact Boolean/integer literals across phi inputs and evaluates equality/inequality only; unknown updates clear the binding. No arithmetic or iteration-count inference. |
| Repeated compound parameter guard | 1 geesefs | Carry at most eight exact parameter/constant comparisons. Mutable field loads and computed loop values are not stable facts. |
| Checked successful return | 1 gmqtt | An exact error result whose nil edge dominates the return satisfies the existing held-for-caller success contract; a shared merge successor does not establish nil. |
| Conditional caller release | 1 fortio | A private non-escaping helper's constant Boolean result identifies held state, and every bounded direct caller releases the same global mutex on that result branch. |
| Imported possible writer guard | 2 gocryptfs | A matching opaque wrapper call and dominating deferred standard exclusive unlock establish possible-held-writer uncertainty, not acquired-writer or protected-field proof. |
| Repeated loaded loop guard | 1 gonc | Matching loaded Boolean guard/polarity around a possible exact release makes same-site loop reentry unknown; it does not prove stable fields or loop completion. |

The caller scan includes generated source bodies and package initialization,
limits each helper to 32 direct synchronous calls, and stops after 20,000
instructions. Exhaustion or function-value escape cannot establish a contract.
The result must be used unchanged in the caller's immediate branch; looping
call sites, modified flags, mismatched mutexes, and a leaking second caller do
not qualify. The first implementation deliberately handles only package-global
`sync.Mutex` values, not receiver binding or reader/writer mode contracts.

The imported writer boundary follows the existing rule that a separately owned
writer lock makes field protection uncertain. It does not infer that an
unrelated exclusive lock protects the written field. That known coverage loss
remains explicit. Known-empty imported calls, another receiver, writes before
the defer, and explicit early releases remain diagnostic. The gocryptfs writer
is `ContentLock`, not the distinct `fdLock` reader guard.

Loaded loop guards are also uncertainty rather than safety proofs: genuinely
changed fields can leak a lock, and those cases remain a documented coverage
gap. Different guard fields, opposite polarity, and two acquisitions within
one guarded region retain reports. Repeated proof states now deduplicate a
recursive diagnostic by acquisition instruction and lock identity without
discarding the states or suppressing another lock's diagnostic.

Minimized accepted and diagnostic forms accompany every boundary. Unit coverage
also verifies that entering a phi from an unknown predecessor clears an old
constant without mutating the predecessor state. Structured trace assertions
cover the new feasibility, proven caller-release, and uncertainty reasons.

## Active remainder

Fourteen reviewed false positives are still reported; they are work to continue,
not blockers or claims that the code is defective:

- Nine contextual order-cycle cases: initialization before publication,
  fresh versus already-published owners, join-before-reverse-order, distinct
  peer objects, and common caller serialization.
- Two template parse error paths need stable initializer evidence plus a
  bounded documented parser contract; a private variable is not a literal
  constant merely because its initializer is a string.
- ContainerSSH's closed private state domain needs exhaustive transition
  evidence, including the state setter's callers.
- OpenSurge's mutually exclusive prefix/helper conditions need an exact
  predicate relationship, not an action-name exception.
- The gophercloud test depends on callback invocation cardinality; no loop-count
  guess was added.

## Bounded cost and incomplete results

Extending branch-state retention exposed unnecessary exploration of functions
without lock acquisitions. An intermediate integer-literal build spent over
20 seconds on an isolated `image/png` scan and reached about 7 GB during a
canonical dependency scan. That intermediate build was stopped, not counted
as a successful replay.

The flow now skips functions without direct or bound summarized acquisitions,
including the same TryLock and reader modes as the authoritative mutex model.
Each remaining function has a 4,096-state limit. Diagnostics and new order
edges remain private until completion; exhaustion discards both and records
`lock-state-budget-exhausted` as unknown. It does not infer a caller-release
contract from an incomplete return set. This intentionally loses findings in
complex functions rather than retaining partial proof claims.

A regression fixture exhausts the budget after an early recursive acquisition
and an order edge between distinct mutex fields. Neither diagnostic nor edge
escapes into a later function. The shared diagnostic buffer additionally tests
abandonment, commit idempotence, ordering, and preservation of prerequisite
results. The final isolated `image/png` scan took 1.60 seconds / 143,724 KB
maximum RSS, compared with baseline 1.97 seconds / 162,492 KB on this machine.
The final uhaha trace confirms carried-literal branch pruning in
`runPromotionWatcher`, not budget exhaustion. Its standard-library dependency
`reflect.StructOf` does exhaust the budget and is deliberately inconclusive.
None of the four audited true-positive controls was lost.

## Validation and provenance

Baseline source was `5074635` (audit-only changes after the implementation
contained in the final previous binary). Immutable baseline:
`.build/gohawk-followup78-goroutines-v5`, SHA256
`5516cad4c83bffd8dca28713df53f8d3d1a463b838c23d302da9e10ddc257419`.

Corrected combined worktree binary: `.build/gohawk-followup207-locks-v11`, SHA256
`d27c4ceb2e08c20f836af935ad7284a8d2a807c0df503bebcac1e7defcf891ff`.
It includes concurrent workers' changes outside lockorder; this record claims
only the reviewed lock sites. It is not stamped as a clean source revision.

Receipts in `.build/followup207-locks-baseline/` and
`.build/followup207-locks-v11/` cover 20 package scopes each. Every run verifies
the original repository revision and uses
`-enable-all -gohawk-include-tests -json .`, `CGO_ENABLED=0`, `GOWORK=off`,
`GOTOOLCHAIN=local`, `GOMAXPROCS=2`, `GOFLAGS='-mod=readonly -p=2'`, and
`PROTO_REPORTER=text`. All 40 scopes loaded successfully. External work was
sequential static analysis only; no candidate tests, applications, or generators
were executed.

The four true-positive controls are ContainerSSH `server.go:246:3` and
`:494:3`, WireGuard `noise-protocol.go:542:3`, and uTLS `common.go:1075:4`.
All are present in both canonical runs. The ledger separately marks the two
previously corrected surf sites rather than counting them among the nine new
corrections.

Focused tests, the focused race suite, and targeted canonical golangci-lint
pass. The root task owns final repository-wide verification. The checkpoint
architecture run found no lock conformance error; two stale references to a
concurrently renamed shared helper still needed documentation updates.

Tier 2 graph verification used `home-james-scratch-gohawk` with source freshness
checks and direct reads after edits. Actual pinned SSA was inspected for the
new boundaries. External pinned checkouts were not indexed. The existing flow
file remains the lock-state orchestration; reporting moved into the existing
operation/obligation file rather than growing that orchestration further. The
operation file exceeds the 400-line review trigger but retains one vocabulary:
mutex transitions and return obligations, including caller-owned critical
sections. No parallel proof engine was introduced.
