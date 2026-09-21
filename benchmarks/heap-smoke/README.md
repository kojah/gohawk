# Local heap modeling smoke test

Evaluation against baseline `190b4d5`, 2026-09-21. This is a feasibility
experiment, not a production analyzer change or a new precision audit cohort.

## Question and scope

Can a small explicit storage map recover useful identity relationships that
`DefinitelySameValueAt` cannot, without mistaking old storage contents for the
current value? The comparison is against this identity helper, not against
all existing ownership/projection helpers or end-to-end analyzer behavior.

The test-only model lives in `internal/ssaflow/store_heap_smoke_test.go`.
Locations are a local allocation plus field/constant-index path. SSA values
carry either a symbolic value or an exact location. Stores replace the value
at that location; loads snapshot its current contents. Pointer aliases use
the same location. It does not infer that different SSA parameters cannot alias.

The model accepts only a single basic block and has an instruction budget.
Unknown writes, calls, captures, slices, dynamic indexes, aggregate copies,
and unsupported instructions stop the proof. Aggregate writes cannot leave
stale child-field entries because they are rejected altogether. This prototype
tracks singleton values or unknown, not sets of possible pointees or CFG joins.

## Results

All 19 hand-written cases pass their separate baseline/prototype expectations.
The test prints the actual SSA for each case, not a hand-reconstructed IR.

| Outcome | Count |
|---|---:|
| Proven by both | 2 |
| Newly proven by prototype | 7 |
| Proven only by existing helper | 1 |
| Neither proves identity | 9 |

The seven gains are replacement of a scalar cell, a struct field, a field
updated through an alias, a nested field, a constant array index, a pointer
reloaded from a cell, and a saved read whose source field is later overwritten.
The lost case is a read-only closure capture that the existing helper already
supports. Stale-value and other-index controls do not become definite matches.
Budget exhaustion never establishes identity.

This is deliberately a small synthetic suite. It does not establish soundness,
real-world diagnostic improvement, or the prevalence of these patterns.

## Cost and reproducibility

The model is 105 lines including comments and imports, separate from the
comparison fixtures and benchmark. It removes **zero** production code today.

On Linux/amd64, AMD EPYC 9645, three runs of the field-alias microbenchmark:

| Query | Time | Bytes/op | Allocations/op |
|---|---|---:|---:|
| Existing helper | 350–399 ns/op | 768 | 6 |
| Prototype | 1,000–1,055 ns/op | 1,073 | 6 |

The existing helper declines this case; the prototype walks the function and
proves it. These are different amounts of useful work, not an end-to-end speed
comparison. Repeated queries currently rebuild the heap; no per-function cache
or amortized performance benefit has been demonstrated.

```sh
go test ./internal/ssaflow -run '^TestHeapSmokeComparison$' -v
go test ./internal/ssaflow -run '^$' -bench '^BenchmarkHeapSmoke$' -benchmem -count=3
make verify
```

Only our own test programs are analyzed. No external repository tests,
generators, or application code are executed by this experiment.

## Decision

**Worth a further bounded experiment; not ready for production migration.**
The gains demonstrate useful local storage tracking with a small core, but
neither reduced overall complexity nor better analyzer diagnostics is proved.

Before adoption:

1. Find reviewed real-world cases benefiting from these exact relationships.
2. Compare against all relevant existing storage helpers, including closure
   and projection proofs, not just the identity query used here.
3. Demonstrate a shadow-mode integration in one analyzer with no new false
   positives, preserving unknown rather than treating it as missing cleanup.
4. Show that the model can replace an existing storage proof rather than add
   another parallel decision path; measure whole-analysis time and memory.

CFG joins, loops, slice backing-store aliases, interprocedural effects, and
escaping objects remain separate proposals, not implied by this result. Do not
expand into whole-program heap analysis on the strength of this smoke test.

## Follow-up: replacement feasibility

The next evaluation keeps production unchanged and adds executable comparisons
in `internal/ssaflow/store_heap_followup_test.go`. The prototype now accepts an
explicit observation instruction and operands, so a probe need not invent a
two-argument `observe` call inside the function being examined. Its supported
instruction set has not been expanded.

### A broader baseline reduces the apparent gain

`storedValueAt` already resolves the scalar replacement case the original
`DefinitelySameValueAt` baseline declined. Thus **one of the seven original
gains is not unique** to the prototype. This is not permission to use
`storedValueAt` as a complete identity proof: its callers supply additional
conditions about how the address is used and when cleanup runs.

The six remaining synthetic field/array relationships still merit attention,
but this follow-up does not establish that all six are unique across every
existing helper, nor that an analyzer currently misdiagnoses them.

### Cleanup and projection probes

The executable follow-up contains:

- Six deferred-cleanup queries: stable capture, latest value, stale earlier
  value, replacement after defer registration, conditional replacement, and
  normal-path Close followed by clearing the deferred cell.
- Two stable-projection queries: an acquired owner's original field and a
  replacement field.
- The latest-store overlap comparison above.

All nine comparisons pass. Existing completion proves the stable capture and
latest value, and does not credit the four remaining queries as guaranteed
cleanup by that defer. The normal-path Close case illustrates an important
distinction: proving whole-function cleanup requires combining paths, not
pretending the deferred action always performs the Close.

Existing projection logic accepts the original field and rejects replacement.
The heap prototype is unavailable at **all eight cleanup/projection probes**:
control flow blocks the deferred cases; the acquisition call blocks the
projection cases. These are availability probes, not a production shadow
integration and not a claim that heap equality itself proves cleanup.

### Connection to reviewed findings

The selected boundaries are relevant to documented real issues:

| Reviewed pattern | Existing machinery | What a replacement would need |
|---|---|---|
| Sidecar re-query before deferred cleanup, round 30 | `storedValueAt` | acquisition effects and deferred observation timing |
| Traefikoidc branch-assigned response, round 31 | `targetStoredOnPath` | path-dependent acquisition and cleanup, not merely unioning pointees |
| Witness transaction cleared after settlement, round 37 | target-or-nil storage plus cleanup flow | relate normal-path settlement to deferred guards |
| MySQL test callback's parent DB cleanup, round 51 | `ProveEnclosingCompletion` | caller bindings, cleanup registration, and lifecycle contracts |

The first three links to pinned source are recorded beside the production
proofs in `flow_stores.go` and `completion_search.go`; their reviewed labels
remain in the named precision rounds. The MySQL test source was also inspected
in the retained batch-48 checkout. Those SQL cases involve callbacks and many
calls, not a straight-line local store/load problem.

This follow-up uses minimized local proof cases and source/ledger inspection.
It does **not** replay those external repositories or measure a new diagnostic
delta. No external test code is executed. No new real-world finding has been
shown to require the prototype.

### Updated decision

**Do not migrate existing proofs or expand this into a general heap engine
yet.** Zero production helpers can be removed on the evidence collected.
The initial speed numbers remain microbenchmarks, not analyzer performance.

Keep the test-only experiment as a reproducible reference. Revisit production
work when an audited field/constant-index false positive supplies a concrete
consumer and a comparison against the full existing proof, not just a weaker
identity helper. At that point, consider a narrow local storage query first;
do not bundle closure semantics, interprocedural effects, CFG solving, and slice
aliasing into one speculative rewrite.

Reproduce both stages with `go test ./internal/ssaflow -run TestHeapSmoke -v`.
