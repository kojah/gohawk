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
