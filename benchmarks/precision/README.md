# Precision regression cohorts

These cohorts preserve human review from repeatable dogfood runs.
Each repository is pinned to an exact revision. The gate fails if a reviewed
false positive returns or a reviewed true positive disappears.

Run both cohorts with `make precision-regression`, or replay one cohort while
reusing retained checkouts:

```sh
scripts/precision-regression.py benchmarks/precision/round-3 \
  --checkout-root /path/to/checkouts
```

Checkout directories use the `owner__repository` naming convention. The
harness analyzes the three shallowest Go modules, matching the original pilot.
Every replay uses `-enable-all`; labels may therefore cover default or opt-in
checks.
New diagnostics are intentionally reported by normal dogfooding review rather
than guessed to be regressions: only a human can label a new finding.

During the 500-repository audit, replay only the new cohort and any directly
affected analyzer controls for ordinary batches. Run the complete cumulative
suite after every fifth batch and at release or CI gates. This keeps routine
audit work incremental while still providing regular integration checkpoints.

Round 4 contains the 20 false positives fixed from a 15-repository traced
audit plus all 23 reviewed true positives. The full audit, including the
remaining labeled noise families, lives in the companion lead-generation
study rather than being encoded as accepted benchmark behavior.

Round 5 preserves the first reviewed batch from the 500-repository `-enable-all`
audit. It covers immediately invoked nested cleanup defers, background process
waiters, and direct `os.Process.Wait`, together with nearby resource and process
leaks that must remain reportable. Only repositories contributing a regression
label are retained in the executable cohort; the broader selection ledger is
kept separately from the CI-sized precision gate.

Round 6 preserves the second reviewed batch. It covers channel identity across
direction conversions, map-sized worker joins, `testing/synctest.Test`
ownership, and test-only API declarations together with nearby lifecycle and
resource findings.

Round 7 preserves the third reviewed batch. It covers proven acquisition-error
branches, custom context implementations, named testing callbacks, and
lifecycle ownership transferred through a returned aggregate. Nearby context,
resource, process, goroutine, and loop-cleanup findings remain reportable.

Round 8 preserves the fourth reviewed batch. It covers cancellation stored in
an enclosing lifecycle, deferred cleanup callbacks, ambiguous `Row` names,
returned testing callbacks, and stop channels received through static helpers.
Nearby resource, process, testing, context, and goroutine findings remain
reportable.

Round 9 preserves the fifth reviewed batch. It covers unused testing handles,
method-value testing callbacks, cancellation owned by a helper goroutine, and
deferred cleanup of an exact resource projection. Nearby test, cancellation,
resource, and goroutine findings remain reportable.

Round 10 preserves the sixth reviewed batch. It covers string fields populated
by external JSON, test cleanup that terminates a launched lifecycle, and
WaitGroup completion signaled only after a worker's substantive work. Nearby
closed-domain, resource, and goroutine findings remain reportable.

Round 11 preserves the seventh reviewed batch. It covers fields reached through
nested JSON aggregates, WaitGroups joined by testing cleanup, feasible error
returns in deferred functions, and canonical production/test syntax. Nearby
closed-domain, evaluation-order, resource, producer, error, and
goroutine findings remain reportable.

Round 12 preserves the eighth reviewed batch. It covers exact typed acquisition
errors, legacy existing-file checks, deferred WaitGroup joins, channel-range
lifecycles, and lock release through directly invoked callbacks. Nearby lock,
resource, process, and goroutine findings remain reportable.

Round 13 preserves the ninth reviewed batch. It covers delayed function-literal
bodies during operand evaluation, cleanup deferred by exact static helpers,
positive singleton-map guards, and close ownership through completely resolved
bound-method callsites. Nearby resource, process, loop-cleanup,
determinism, and goroutine findings remain reportable.

Round 14 preserves the tenth reviewed batch. It covers lock acquisition and
release guarded by the same stable Boolean parameter, plus resource cleanup by
a deferred static helper receiving an exact field projection. Nearby lock,
resource, determinism, and concurrent-capture findings remain reportable.

Round 15 preserves the eleventh reviewed batch. It covers read-only embedded
byte data, deliberate prior-error precedence, and independent error-producing
calls that consume the same ordinary payload. Nearby global-state,
closed-domain, resource, error-ownership, and determinism findings remain
reportable.

Round 16 preserves the twelfth reviewed batch. It covers empty private
interface-marker methods and injective constant-map reverse lookups. Nearby
testing, global-state, resource, lock, process, loop-cleanup, and ordered-output
findings remain reportable.

Round 17 preserves the thirteenth reviewed batch. It covers WaitGroup joins
owned by exact deferred callbacks, including `sync.OnceFunc`, and keeps nearby
resource, error-ownership, and early-completion findings reportable.

Round 18 preserves the fourteenth reviewed batch. It covers cancellation
handed to a launched static helper and WaitGroup ownership returned through an
exact join callback. Nearby process, resource, and error-ownership findings
remain reportable.

Round 19 preserves the fifteenth reviewed batch. It covers exact cleanup
exported through a deferred lifecycle callback and suppresses missing-unlock
claims when a dominating earlier defer may release that exact lock. Nearby
resource, process, loop-cleanup, and termination findings remain reportable.

Round 20 preserves the sixteenth reviewed batch. It distinguishes stable
address identity from stale value snapshots and recognizes exact Testify
`assert.NoError` success guards. Nearby resource and goroutine findings remain
reportable.

Round 21 preserves the seventeenth reviewed batch. It covers exact optional
resource acquisitions that merge with nil and stable standard-library string
replacers used only through their documented operations. Nearby resource,
process, global-state, and determinism findings remain reportable.

Round 22 preserves the eighteenth reviewed batch. It covers optional local
workers joined beneath the exact stop-channel guard and imported cleanup facts
applied to an exact stable resource projection. Nearby resource and
process-termination findings remain reportable.

Round 23 preserves the nineteenth reviewed batch. It covers a completion
channel read from a type-asserted event, a started command returned as a
by-value copy, and response bodies handed to an unexported helper that returns
an owning iterator. Nearby resource findings on unauthorized, error, and
temporary-file paths remain reportable.

Round 24 preserves the twentieth reviewed batch. It covers a cancel function
stored into a local that a deferred guard closure captures and a ticker stopped
inside a closure launched by `sync.WaitGroup.Go`. Nearby resource, process,
and defer-in-loop findings remain reportable.

Round 25 preserves the twenty-first reviewed batch. It covers a test helper
that registers `testing.Cleanup` for its file argument, `os.IsPermission`
switch cases on an acquisition error, fatal `require.Error` and
`require.NotNil` assertions after any acquisition, a returned struct literal
carrying a captured method value, and buffered result channels stored in a
slice. Nearby resource, goroutine, and inline-error findings remain reportable.

Round 26 preserves the twenty-second reviewed batch. It covers a worker that
signals completion through an element of a captured slice the parent returns,
and a started command whose os.Process is returned to the caller. Nearby
ticker, temporary-file, response, and compression findings remain reportable.
