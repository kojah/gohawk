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
