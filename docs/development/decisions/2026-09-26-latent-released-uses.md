# Latent released uses, reported at the triggering call

Decided 2026-09-26.

A function that releases a parameter and then operates on it has a defect
whoever calls it, and gohawk reports it at the operation. A function that does
so only when one of its own Boolean parameters selects the release is correct
for callers that pass the other value, so reporting it would blame the
function for a choice its callers make. That defect is latent, in Infer
Pulse's sense: the lifecycle summary records it with the condition, and only a
call whose constant arguments satisfy the condition reports it.

The summary claim, a released use, is structural and uses the same
`ssaflow.CallCondition` as every other conditional claim: under the condition,
a direct cleanup call on the exact parameter dominates a method call on it,
with nothing else touching the parameter in between. Which methods fail on a
released value is the analyzer's contract, so the fact stays neutral and the
invalidation table stays with `resourcelifetime`.

Pulse reports a bug whose path depends only on the function's own tests as
manifest. gohawk does not: a release on a branch the function decides by its
own data does not dominate the use, and the use is not claimed. That keeps a
reported use wrong on every path that reaches it, at the cost of the bugs that
depend on data the analysis cannot decide.

Still excluded: a helper that releases or uses the parameter, a released use
reached through a chain of calls that forward the constant, and conditions on
anything but Boolean parameters.
