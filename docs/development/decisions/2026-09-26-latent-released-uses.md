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

A helper counts on either side: the completion engine's exact proof that it
closes the parameter on every return, under the constants the case binds,
makes it the release, and a summary requiring a method of the parameter on
every path makes it the use. Any other call that receives the parameter still
cancels the claim. A function that forwards its own flag to a callee with a
latent released use has the same latent use under its own condition, so the
defect is reported at the call that finally fixes the flag; a callee's latent
use that the call's literals trigger alone is that call's defect and is not
composed.

Still excluded: conditions on anything but Boolean parameters. A nil
condition needs a walk that assumes a parameter is nil; an integer, string,
or enum condition needs equality in the shared vocabulary and multiplies the
cases a function exports, which waits for a real-world pattern to justify it.
