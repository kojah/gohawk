# resourcelifetime design notes

The public page is [resourcelifetime](../../analyzers/). This note keeps every precision
boundary: what the analyzer accepts or reports at the edge of its proof, and
why. Update it with the fixtures when a boundary changes.

## Detection boundaries

Release owned resources on every path. Storing a resource in a partially
constructed object does not transfer ownership when an error path returns
without that object.

The built-in contracts cover files, transactions, SQL rows and statements,
HTTP response bodies, and gzip/zlib writers. Compression readers do not own
their inputs, and their Close does not finalize or validate the input stream,
so they are not cleanup obligations. The underlying file still needs closing.

HTTP bodies come from `http.Get`, `http.Post`, `http.PostForm`, and the
`Client` methods `Do`, `Get`, `Post`, and `PostForm`: net/http documents
"Caller should close resp.Body" for each. `Head` in either form carries no
such sentence and usually returns `http.NoBody`, so it is not an acquisition.
A project type named `Client` with a `Get` method is matched by package path,
not name, and is not an acquisition (`http_client_methods.go`).

A direct `http.NewRequest("HEAD", ...)` used only by `Client.Do`, with a
fresh zero-value client also used only by `Do`, has an uncertain body
acquisition. The usual transport returns `http.NoBody`; the replaceable global
transport prevents treating this as proof of no resource. Explicit client
timeouts, custom transports, request mutation, and helper escapes are outside
this narrow boundary and retain the ordinary cleanup obligation.

An exact `http.Get` targeting an unchanged local `httptest.NewServer` can also
have uncertain body acquisition when its handler and visible helpers only set
non-framing headers, cookies, or a non-redirect status. Writer escapes, body
writes, dynamic or framing headers, and visible default-client or transport
overrides exclude this boundary. `server.Client().Get(...)` on the same
server qualifies too: httptest gives that client a transport to the server
and no timeout. Any other use of that client, such as setting `Timeout`,
and any other client, exclude it (`http_servers.go`). Mutations of global HTTP defaults hidden in
other packages remain an accepted coverage gap; this is not proof that a body
can never need closing.

A returned wrapper that holds the resource, proven by the `ReturnedOwner`
summary of every constructor in the chain, as `log.New`, `slog.New`, and
`slog.NewTextHandler` hold their argument, is a handover when the constructor's
own summary claims a retaining result; the caller then owes the resource and
is reported where it drops the wrapper. A proven chain without that claim,
including any unexported constructor, and a wrapper returned inside an
aggregate, are uncertain boundaries. Discarding the wrapper on an error return
still abandons the resource. See `retaining_results.go` in the fixtures.

For compression writers, error returns and explicit pipe aborts can abandon
the output rather than publish it. Those paths are uncertain, not proven
finalization; successful returns still require Close where the contract applies.

The false edge of the tracked `database/sql.Rows.Next` call is also uncertain:
the final result set closes automatically, but another result set may remain.
An early break or return before exhaustion is not covered by that boundary.
By contrast, `Rows.NextResultSet` returning false closes the exact Rows value.
This result-conditioned release can pass through a forwarding helper and its
cross-package lifecycle summary, or a visible straight-line owner method that
forwards the call through an exact field. The field may hold Rows through an
interface only when its concrete value is proven to be that same `*sql.Rows`.
A true result, another field or Rows value, opaque dispatch, and a wrapper
whose field relationship is unavailable do not prove release.

Closing the exact parent DB, directly or through a dominating defer, suppresses
missing-release findings for DB-prepared statements. It does not discharge
rows, transactions, or statements prepared through a Conn. Parent closure is
not treated as immediate statement invalidation.

Committing or rolling back the exact parent transaction also makes active row
cleanup uncertain: transaction-context cancellation closes its rows, including
rows queried through a statement prepared on that transaction. A dominating
deferred finish is recognized; a different transaction or a finish confined
to only some return paths is not. This does not treat `DB.Close` or
`Stmt.Close` as closing active rows.

Passing a wrapper around an aggregate that contains the exact resource to a
callee that may retain it can make ownership uncertain, not prove cleanup.
This includes `io.MultiWriter` installed as logging output. The wrapper must
itself potentially retain the aggregate; an ignored input or read-only
transformation does not establish this boundary. Discarding the wrapper does
not by itself transfer ownership.

A longer chain of constructors that each keep their argument, such as
`slog.New(slog.NewTextHandler(io.MultiWriter(os.Stderr, file), nil))`, hands the
resource over only where a callee is proven to store it: `slog.SetDefault`
installing the logger as the process default, for example. Storing such a
chain into a field or element of an object the function did not allocate, or
into a package variable, such as routing a logger into a server's `ErrorLog`,
hands it to that object. A chain the function only uses, such as a logger
whose `Info` it calls, or one stored into a local allocation, leaves the
resource owed. Up to four constructors are followed.

DB acquisitions through `PrepareContext`, `QueryContext`, and `BeginTx` are
known to fail when the exact context from `WithCancel` or `WithCancelCause`
was synchronously canceled before the call. Conditional, deferred, or concurrent
cancellation does not establish this, nor do timing or test assertions.

A successful `errors.As` match on the exact acquisition error also establishes
failure. Matching an unrelated or joined error, or failing to match a type,
does not prove that the resource was never acquired.

A visible boolean error helper can establish the same failed-acquisition
branch when every normal return for the exact nil error is literally false.
This includes callbacks passed through immutable lexical captures. All capture
origins must agree, and one shared work budget bounds the proof. Replaced or
escaped callback cells, rewritten errors, recursive predicates and deferred
result mutation do not establish that implication.

A called closure that guards an exact captured HTTP response's `Body` before
closing it is an ownership uncertainty boundary. The guard and `Close` may
read different loads, so this does not prove release. The response cell must
still hold the acquisition, with no visible pointer escape or replacement;
extra Boolean conditions and cleanup of another response do not qualify.

Channel timers and tickers are not cleanup obligations: since Go 1.23, the
garbage collector can reclaim them without `Stop`. Missing `Stop` alone does
not establish a leak. This check does not infer legacy main-module or
`GODEBUG=asynctimerchan=1` settings, nor claim that retained workers or
`AfterFunc` callbacks are harmless.

An exported constructor that returns a wrapper proven to hold the resource it
opened, such as a `*slog.Logger` over a log file, hands the resource to its
caller. The wrapper cannot release it, so the caller must keep the wrapper,
hand it on, or return it; a caller that drops it is reported with "resource
held by the result of ... is dropped on some return path". A wrapper that only
may hold its input, such as `bufio.NewWriter`, is not treated this way.

A resource that program exit reclaims is not reported. The acquisition must be
in `main.main` of package `main`, outside any loop or closure, in a package
that never calls its own `main`. It then runs at most once, and every way out
of `main` ends the process, which closes the file, response body, rows, or
statement. Cleanups with an effect that exit would lose are still reported:
compressors must flush, transactions must commit, and an inferred owner's
`Close` is not assumed to be effect-free.

## Contracts, transfers, and use after release

The `owned` contract family is not a table. A constructor in another package
whose returned struct holds a resource it acquired itself, and whose type has
a method that releases that field on every return, is summarized by the
lifecycle facts pass, and callers of that constructor then owe the method
exactly as they owe `Close` to `os.Open`. A wrapper that stores a caller's
resource, or a type whose methods never release the field, produces no
contract.

Two process and type boundaries are modelled rather than guessed. Files
listed in `os.ProcAttr.Files` and handed to `os.StartProcess` reach a call
with no visible body inside an aggregate, so the child's inheritance of the
descriptors is an opaque boundary and nothing is reported. A comma-ok
assertion of the resource to its own type, or to an interface it satisfies,
cannot fail for that value, so a release under the assertion is a release,
in the caller and inside a helper alike; an assertion the resource does not
satisfy leaves the release conditional.

A helper that releases every element of what it was handed inside a loop,
whether it ranges over a slice, an array, or a receiver field, is an
uncertainty boundary rather than a missing release: the completion search
reports that its only release lies inside a cycle, and the caller is not
reported. An imported helper carries the same loop as a may-claim in its
summary. A helper whose release depends on a flag has complete path
information and stays diagnostic, unless the call fixes the flag. A call
passing a constant Boolean, or a caller parameter that the caller's own
call fixed, binds the helper's parameter, and the completion search follows
only the branch that constant selects: `finish(file, false)` releases when
the helper closes under `!keep`, and `finish(file, true)` proves the leak.
The binding reaches a flag the helper tests inside a deferred closure through
the captured cell, when that cell is written once before capture and only
read after. An imported helper carries the same answer as argument cases in
its summary, bounded to two guarding Boolean parameters; a helper with more
keeps only its unconditional claims, so a constant call through it stays
diagnostic. A deferred call is bound too, so `defer finish(file, true)` is
reported rather than accepted as a release somewhere in the helper. A
variable flag, a flag merged from two branches, or a comparison of the flag
decides nothing. Fixtures: `resourcelifetime/argument_cases.go`.

A helper that acquires a resource and hands it straight back as a result,
such as `OpenConfig(path) (*os.File, error)` or one returning an
`io.ReadCloser`, is summarized the same way, and its callers owe the result
type's cleanup. That claim requires the value to reach the return untouched:
a helper that also registers it, calls a method on it, captures it in a
returned callback, or may have closed it before returning makes no claim,
so a dropped result from such a helper is not reported. Standard-library
packages the contract catalog already models keep the catalog's decisions;
an inferred owner never adds to them.

Fresh acquisition inference requires a known concrete resource type, in a
field or as the value returned. A custom `Close` method alone does not
establish acquisition: the result may be a lazy or empty handle. Nested
custom owners remain unknown, so leaks through those constructors may be
missed.

The same summaries decide the other direction. A call that stores the caller's
resource in its returned struct transfers the obligation only when that
struct's type has a method proven to release the field; a returned view such
as a buffered reader leaves the obligation with the caller, even when the
view's type has a `Close` of its own that releases nothing.

Every instruction after an acquisition is classified once as a release or
transfer, an opaque use, or nothing, and a leak is reported only when no
release or transfer covers a return and nothing opaque consumed the resource
on the way there. Summaries are the analyzer's knowledge: a summarized callee
that neither releases, stores, nor owns the resource is transparent, so a
read helper leaves the obligation in place. Opaque uses are real boundaries:
the resource handed to an interface method or function value, to a callee
with neither body nor summary, to a launched or deferred literal without a
proven release, or into a channel, map, or append the analyzer does not
track. Past such a use the analyzer stays silent rather than guess.

The same uncertainty applies when a retained aggregate argument contains the
resource, or a helper's aggregate result is published through a global. An
imported helper that receives the aggregate is judged by its summary's
kept-contents claim: one that keeps, sends, starts, returns, or hands to an
opaque callee anything loaded from the path where the resource sits, or that
has no summary at all, stays a boundary, while one proven to keep nothing at
that path leaves the obligation with the caller. A helper
that closes a merged resource argument or a body loaded from a merged response
also makes the result unknown: cleanup of the selected value does not prove which
acquisition it released. These boundaries do not establish ownership or cleanup.
Read-only helpers, overwritten response bodies, and cleanup of unrelated values
do not qualify for this merged-value boundary.

A deferred closure registered before acquisition may close a captured variable
assigned later. When that defer dominates the acquisition and its body contains
cleanup derived from the captured cell, the result is unknown even if several
acquisitions feed the cell. This does not prove which value is closed and can
miss leaks caused by overwriting the cell. Read-only captures, unrelated
cleanup, and deferred arguments evaluated by value do not establish this boundary.

Compression writers over a local in-memory buffer are exempt, including
exact `bytes.NewBuffer` and `bytes.NewBufferString` results. Leaving one
unclosed holds nothing outside the function. Never closing it before the
buffer is read truncates the output, which is a data defect rather than a
leak and is not this check's claim.

The core `use-after-release` check is the dual of the leak check. After a
plain (not deferred) release of the acquired value, it reports an operation
the API documents as failing on a released value, such as a write to a closed
file, a scan of closed rows, a statement on a committed transaction, or a read
of a closed response body. The release must dominate the use, so a release on
one branch followed by a use after the merge is not claimed, and only the exact
acquired value counts. Local fields, constant array and slice elements, saved
aliases, and agreeing branch assignments preserve that identity. Replaced
values, mixed branch assignments, and mutated response bodies do not.

A helper that performs the operation counts as the operation: a call that
hands the released value to a function whose summary says it calls `Read`
on that argument on every path is a read of it, and the diagnostic names
the helper. A helper that reads on some paths only, or that reads through
a reader it built around the value, requires nothing and stays an opaque
use.

The proof stops at opaque effects, resource mutation, escaping ownership,
asynchronous exposure, or its search budget. A writer reset after Close is
therefore not mistaken for continued use of the closed stream. The check does
not cross goroutine or loop-iteration boundaries, infer releases inside helpers,
or infer invalidation from arbitrary methods named Close. Compression reader
cleanup alone does not prove that a subsequent read fails.

Harmless idioms such as `rows.Err()` after `rows.Close()` or `Rollback` after a
failed `Commit` are not reported. Double-close is deliberately not checked.
Compression writers over in-memory buffers are checked for use after release
even when they are exempt from the missing-cleanup check.
For transactions, Commit must have succeeded on the path to the later use;
an unsuccessful commit attempt alone does not establish invalidation.

## Former public summary

Reports resources that are not released on every return path, and operations
on a resource after it has been released.

The built-in contracts cover files, transactions, SQL rows and statements,
HTTP response bodies, and gzip/zlib writers. A constructor in another package
that acquires one of these and returns it, or returns a struct with a method
that releases it, is inferred as an owner, so its callers owe the same cleanup.

A resource's obligation ends when it is released, returned, stored somewhere
that outlives the function, or handed to a callee proven to keep it. An
exported constructor that returns a wrapper holding the resource, such as a
`*slog.Logger` over a log file, hands the resource to its caller, which must
keep or pass on the wrapper. When the resource reaches code the analyzer
cannot see through, such as an interface method or a callee without a
summary, nothing is reported.

Some cases are deliberately not reported:

- channel timers and tickers, which the garbage collector reclaims since Go 1.23;
- compression writers over an in-memory buffer;
- a file, response body, or rows value acquired once in `main.main` of package
  `main`, which program exit closes. Compressors and transactions there are
  still reported, because exit would lose their flush or commit.

`use-after-release` reports an operation documented to fail on a released
value, such as a write to a closed file or a scan of closed rows, when the
release dominates the use. Double-close is not checked.
