# deferinloop design notes

The public page is [deferinloop](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

Storing the resource in a containing aggregate makes its iteration-local
lifetime unknown, including a collection populated before the defer. Such
collections may deliberately keep resources for use after the loop. This
boundary does not prove the collection is eventually closed.

A deferred call is a candidate only when it has a receiver: a `sync` lock
acquired in the iteration, a value whose type is in the lifecycle resource
vocabulary, or the result of a constructor whose facts prove it owns a
resource. A deferred function value such as `cancel` has no receiver and is
not considered.

A deferred `Body.Close` on a `*net/http.Response` acquired in the iteration is
that response's cleanup. The response is the body's owner, so storing,
appending, or handing off the response makes the lifetime unknown just as it
would for the body, and closing any load of the same response's `Body` in the
iteration settles it. A store into the `Body` field anywhere in the function
declines the candidate: the defer may then close a value the acquisition did
not produce. A project type with a `Body` field carries no contract. The
fixtures are in `response_bodies.go`.

## Final lifetime proof

`proveDeferLifetime` returns one structured state, reason and witness after
its existing traversal. Reporting and final tracing consume that proof. A
live-backedge witness is rejected and reported even if another path is opaque.
If no live path wins but an unknown resource reaches a backedge, the final
outcome is unknown. A return-only or settled path can establish no live
backedge; this does not claim that every resource was explicitly closed.
Retention before the defer and an unavailable SSA instruction location also
remain unknown. The iterator exhaustion rule, successor feasibility and first
live witness retain their existing behavior.

The trace regression runs with the existing analyzer fixture suite and covers
return-only acceptance, unknown/settled joins, opaque/live joins and inner
loops that may not run. The cleanup-on-both-branches fixture currently reaches
an unknown backedge, so its silence must not be labelled proven cleanup.
Final reasons now distinguish `no-live-backedge` from
`lifetime-unknown-at-backedge`; they replace the former combined accepted
`settled-or-unknown-before-backedge` reason. Evidence events remain beside the
classifier, while one final proof decision is emitted by the entry point.

## Trace presentation cost

Candidate and instruction evidence build detail maps and format SSA values only
when the candidate probe is enabled. Required alias, containment and lifetime
queries remain outside those guards. Final presentation was already guarded;
tracing preserves the authoritative proof and all enabled event contents.
The existing trace fixtures cover accepted, unknown and reported backedges.
