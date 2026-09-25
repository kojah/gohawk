# Exact loop counts, compared with proven obligations

Decided 2026-09-25. Implemented in `412e9b0`.

An analyzer must not guess how often a loop runs. The one permitted
loop-count argument takes an exact count from `ssaflow.ProveCountedLoop` or
`ssaflow.ProveCountedRegion` and compares it with obligations the analyzer has
already proven, such as receives against single sends.

The first use is the counted select drain in `goroutineownership`: a loop that
runs a blocking, receive-only `select` exactly N times over channels made in
the function, each with at most one send, must have received every send when
it exits and N covers every channel. The proof counts messages rather than
paths, so it never unrolls the loop.

Still excluded: dynamic bounds, a `break` or `return` out of the loop, `default`
or send arms, and a bound carried through a struct field, as in the 115driver
finding from batch 60, which is a value-provenance question rather than a loop
count.
