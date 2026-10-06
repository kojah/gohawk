# Summary cases: bounded, positive, and selected by the call

Decided 2026-09-26.

A lifecycle summary may hold cases: positive cleanup guarantees that hold
under a condition a caller can check, on a result or on Boolean arguments
fixed to constants. This follows Infer Pulse's disjunctive summaries in shape,
but not in approximation. Pulse drops disjuncts when it has too many, which is
sound for reporting a bug it found and unsound for the other half of gohawk's
facts, the claims that suppress a diagnostic because a callee is proven to
clean up. So a gohawk case is only ever a positive guarantee proven on every
normal return under its condition, a missing case is unknown, and a function
whose cases would exceed the bound keeps exactly the unconditional summary it
had before.

The bound is structural. Result conditions cover at most four result slots, as
before. Argument conditions cover at most two guarding Boolean parameters, so
at most eight assignments. Cases need no solver: a condition is either a
result outcome the caller tests or a constant the call passes, and a branch is
decided only when it tests the bound value itself or its negation.

The result-conditioned records that existed before are cases with no argument
condition. There is one list, one proof, `lifecycle.ProveCompletionForCase`,
and one condition type, `calls.CallCondition`, with one matching rule,
`Matches`. Every other summary that holds under a condition moves onto the same
type, so there is one vocabulary to prove, export, and match; the completion
search's own result-test enum was the first duplicate removed.

The first uses are `resourcelifetime` and `cancellationownership`: a helper
that cleans up only behind a flag settles the resource at a call passing the
constant that selects the cleanup, locally and across packages, and a deferred
call passing the other constant is reported. Binding the callee's branches
also exposed a completion-search mapping that credited an unlock of one mutex
as releasing a sibling mutex of the same owner; the target now maps as the
mirrored field, and `lockorder` reports the order that was lost.

## One condition vocabulary

Every claim of the form "X holds when Y" now names Y with
`calls.CallCondition`: cleanup cases and the unconditional discharges share
one `Discharges` list whose empty condition is the Must claim; synchronous
invocation is a discharge of its own method; result facts are result cases,
an outcome of one result under a condition that may name a parameter's
nilness or a paired error's outcome; and `lockorder` names the result a lock
is held under the same way. The completion search's private result-test enum
and the result facts' relation kinds are gone.

Two conditional shapes stay separate on purpose. The concurrency facts' path
alternatives carry a path-condition language over SSA values: comparisons with
integer, string, nil, and Boolean constants, scoped by the call sites a path
was bound through, and implied facts about what a helper returned. They select
feasible effect sequences while summaries compose, rather than guaranteeing a
claim, and only a small subset overlaps `CallCondition`; folding that subset
would split one condition across two representations, and folding all of it
would load opaque branch identities into the shared claim condition. The heap
projection's must-hold flag is judged on every return with a non-nil result
for every hold alike, so it is not a condition a caller selects.

Argument conditions also fix a nilable parameter to nil or non-nil: a nil
literal, or a value never nil by construction, decides the callee's nil
comparisons of that parameter. Nilness is bound only for a parameter the
callee compares with nil or captures, so the bindings, and the memo keys that
name them, stay few.

Still excluded: comparisons with integer, string, or other constants, and
relations between arguments.

## Why not full disjunctive summaries

A summary is one postcondition joined over every path, with must and may
marks, plus the bounded cases above. Full per-path disjuncts, as Infer Pulse
keeps them, were considered and not adopted, for these reasons.

- Facts are read in both directions. A claim that a callee releases a value
  suppresses a diagnostic in the caller. Pulse bounds its disjuncts by
  dropping some, which only loses bugs when a disjunct serves to report one,
  but a dropped disjunct that carried a release makes the caller report a
  leak that is not there. gohawk therefore exports only guarantees proven on
  every normal return under their condition, and past the bound it keeps the
  unconditional summary rather than a truncated set.
- A disjunct is useful only when the caller can tell which one applies.
  Pulse selects them with path conditions and an arithmetic solver. gohawk
  has no solver, so a case is keyed only by what a call site can see: a
  result it tests, or a constant it passes. A disjunct keyed by the callee's
  internal branch history could not be matched and would join back anyway.
- Summaries cross packages as analysis facts, written for every function of
  every dependency. Disjunct sets multiply fact size and the cost of applying
  a summary at every call site, where the structural bound (four result slots,
  two guarding Boolean parameters) keeps both small.
- Choosing which disjuncts to drop is an order-sensitive policy, and results
  must not depend on evaluation order.
- The precision it leaves out is correlation the caller cannot observe, such
  as a helper that closes a file on one internal branch and returns it on
  another. That becomes may on both sides, and the caller's obligation is
  unknown: a stable false negative rather than a guess.

None of these costs Pulse precision, because Pulse reasons by
under-approximation: each disjunct describes executions that can really
happen, a report cites one of them, and no disjunct claims that something did
not happen or happened on every path. Dropping a disjunct, or keeping a
different set under another order, leaves every remaining disjunct a true
path, so it can only lose bugs. A caller combined with a callee disjunct that
did not release is on a real path, so the leak it reports is real, and
Pulse's solver discards combinations whose path conditions contradict the
caller. gohawk's reports also need positive evidence of a violation, but its
suppressions rest on over-approximate guarantees about every return, and a
guarantee cannot be kept in part. Each concern above is a recall cost for
Pulse and a false-positive risk for gohawk.

Real disjuncts would fit an analysis whose claims only report and never
suppress, with a solver to select them, which is a different policy from the
one gohawk's trusted claims follow. The concurrency facts' path alternatives
are the one disjunction-like shape in the model, and they select feasible
effect sequences rather than guaranteeing a claim, as described above.
