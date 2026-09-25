# producerlifecycle design notes

The public page is [producerlifecycle](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

Complete shared concurrency summaries expose sends and receives hidden in
helpers, including imported helpers. They feed the existing bounded count proof:
a local unbuffered channel, an observed receiver, and explicit excess sends.
Deferred receiving helpers count as receivers too. Opaque channel consumers,
possible drain loops, and additional receiving goroutines leave the result
unknown rather than being mistaken for absent receivers. Incompletely modeled
competing workers can therefore suppress a finding, unless shared call-effects
evidence proves that all uses of the channel are non-receiving. Buffered channels and
externally supplied channels remain outside this proof.

Loop-based send counts remain unknown: a repeated statement does not prove
multiple sends are feasible. This deliberately misses unbounded producer loops
until their excess production can be established without a cardinality guess.
Callers that terminate the process instead of returning do not establish an
abandoned receiver lifecycle.

### Sends after a service loop stops

The experimental `stopped-loop-send` check covers a long-lived variant. A
struct owns an unbuffered channel, a background goroutine serves it from a
`select` loop, and that loop returns when a stop channel or its context fires.
After it returns, a plain send from an exported method has no receiver and
blocks forever.

The check only reports when the package proves every part of that shape:

- The channel is an unexported field of a struct declared in the package, so
  the package sees every use. It is made unbuffered, and it is never closed,
  supplied from outside, copied, or handed to other code.
- Every receive is a `select` arm in a loop of a function started only by `go`,
  and the same `select` has another arm that returns. That stop arm must be
  able to fire: the package sends on or closes the stop channel, or it is the
  `Done` channel of a context that is not `context.Background` or
  `context.TODO`.
- The send is a plain send in an exported function or method. It is not in a
  `select` with another way out, not inside the loop itself, and not in the
  function that starts the loop.
- A branch on the owner's own state before the send, such as a `running`
  flag, protects it only when one owner mutex is held for the check and the
  send, every write of the flag holds that mutex, and every stop signal does
  too. A flag read or written without the lock, or a loop that can also stop
  on its context, does not protect the send, so it is still reported. A guard
  this package cannot read, such as a method call, is not reported.

Buffered channels, unexported helpers reached through guarded entry points,
and loops that never return are not reported.

### Ranges that wait on a failed producer

The experimental `unclosed-range` check covers the receive side. A range over
a channel ends only when the channel is closed. When the method that closes it
does so only on success and returns an error without closing, a goroutine
ranging over the channel while that method runs waits forever after a failure.

The check only reports when the package proves every part of that shape:

- The channel is a field of a struct declared in the package that no other code
  can reach: an unexported field, or any field in a `main` package, which no
  other package can import. It is made in the package and never handed out;
  it may be buffered, since a range waits for the close either way.
- The range has no other way out: no `break` or `return` leaves it.
- Exactly one method closes the channel, on its own receiver, and none of its
  closes is deferred. The method returns an error, and every return it can
  reach without closing yields an error proven non-nil: an `errors.New` or
  `fmt.Errorf` result, a boxed value, an error the path tested against nil,
  or a context's `Err` or `Cause` after its `Done` channel fired. A return of
  nil without closing is a step that a later call may finish, so it is not
  reported.
- The method is never called inside a loop, where a retry could close the
  channel, and one goroutine calls it on the same object that is ranged over
  concurrently, reached through a variable written once.

A select with other arms, a closer reached through an interface or function
value, and a producer in another package are not reported.
