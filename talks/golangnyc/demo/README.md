# Live demos: resource leaks and concurrency

Allow about two minutes: show the code, run Gohawk, explain the diagnostic,
then fix the leak and run it again. Both versions fit on one screen.
This is a separate Go module so the other intentional talk defects do not
appear in the opening demo.

Then spend another 30–45 seconds each on HTTP and database cleanup to show
that the same check covers more than files. Keep the full sequence to about
three or four minutes by showing the prepared fixes for these two examples.

## Prepare

Build the current checkout once, before presenting. From the repository root:

```sh
agent-test go build -o .build/golangnyc-gohawk .
cd talks/golangnyc/demo
export GOWORK=off
```

On Xenia, `agent-test` is `/home/james/.local/bin/agent-test`. Keep the
editor and terminal font large enough for the room. Open `before/header.go`.
The functions are for static analysis; no sample file or running program is needed.

## Rehearse

1. **Show the function (20 seconds).** “This reads the first sixteen bytes of
   a file.” Give the audience a moment to look at it.
2. **Explain the mistake (25 seconds).** A short file makes `io.ReadFull`
   return an error. That return skips `f.Close()`. Repeated calls can leave
   file descriptors open until cleanup occurs; enough open descriptors can
   prevent the process from opening more files.
3. **Run Gohawk (35 seconds).**

   ```sh
   agent-test ../../../.build/golangnyc-gohawk -enable=resourcelifetime ./before
   ```

   Pause after the output appears. Point to the file acquisition and the
   return that misses cleanup. Expect one `resourcelifetime/missing-release`
   finding and exit status 3. The nonzero status is intentional.

4. **Fix and rerun (40 seconds).** In `before/header.go`, move `f.Close()`
   to immediately after the successful `os.Open` error check and change it
   to `defer f.Close()`. Save and rerun the same command. Expect no findings
   and exit status 0. Pause before the next part of the talk.

   `after/header.go` is the prepared fixed version if you prefer to show it
   without editing live:

   ```sh
   agent-test ../../../.build/golangnyc-gohawk -enable=resourcelifetime ./after
   ```

The read-only example ignores the secondary Close error to focus on cleanup.
Writing or finalizing data may require handling that error too. A clean
Gohawk run means this check found no issue, not that the entire program is correct.

## Show the same problem with other resources

Each example has its own package so you can show one warning at a time.
These functions are compiled and analyzed, not executed: no network access,
database driver, running database, or credentials are needed for the demo.

### HTTP response body

Open `http/before/http.go`. “We get an HTTP response, but if the status is
unexpected, we return without closing its body.” The resource is the response
body; leaving it open can waste resources and interfere with connection reuse.

```sh
agent-test ../../../.build/golangnyc-gohawk -enable=resourcelifetime ./http/before
agent-test ../../../.build/golangnyc-gohawk -enable=resourcelifetime ./http/after
```

Show the diagnostic, then `http/after/http.go`: `defer resp.Body.Close()`
immediately after checking the request error covers the early return too.
Closing the body is required; reusing HTTP/1.x connections can also require
reading it to EOF. This example focuses on closing, not a complete reuse policy.

### Database query results

Open `database/before/database.go`. “We query some names and read the first
one. If scanning that row fails, we return without closing the results.”

```sh
agent-test ../../../.build/golangnyc-gohawk -enable=resourcelifetime ./database/before
agent-test ../../../.build/golangnyc-gohawk -enable=resourcelifetime ./database/after
```

Show `database/after/database.go`: `defer rows.Close()` closes the query
results on either path. Unclosed results can keep a pooled connection occupied.
This closes the `*sql.Rows`, not the shared `*sql.DB` pool. The demo checks query
result cleanup; it does not claim detection of every database connection leak.

Each before package should report one `resourcelifetime/missing-release`
finding (exit 3). Each after package should report none (exit 0).

## Concurrency: conflicting lock order

After the resource examples, briefly introduce concurrency bugs, then open
`concurrency/before/locks.go`. Allow about two minutes for this demo.

- Show `Transfer` and `Snapshot`: both need the accounts and ledger locks,
  but they take them in opposite orders. If called concurrently, each can
  hold the lock the other needs and neither can proceed.
- Run the check and pause on the acquisition-order diagnostic.
- In `Snapshot`, move the accounts lock and its deferred unlock before the
  ledger lock and its deferred unlock. Both functions now take accounts first.
- Rerun the same command: the warning disappears. The prepared fixed version
  is in `concurrency/after/locks.go`.

```sh
agent-test ../../../.build/golangnyc-gohawk -enable=lockorder ./concurrency/before
# Expected: one lockorder/contradictory-order finding, exit 3.
agent-test ../../../.build/golangnyc-gohawk -enable=lockorder ./concurrency/after
# Expected: no findings, exit 0.
```

The functions are deliberately small teaching examples, with comments standing
in for the protected work. Analyze them rather than trying to reproduce a live
hang. The warning identifies a potential deadlock if these functions run
concurrently; it does not mean every execution deadlocks. To reset a live edit,
restore the ledger-first order in the before version's `Snapshot` function.

## Reset and check before presenting

To reset after a live edit, move `defer f.Close()` back down to just before
the final return and remove `defer`. Compare `before/header.go` with its Git
diff to confirm the original example is restored.

```sh
GOWORK=off agent-test go test ./...
GOWORK=off agent-test ../../../.build/golangnyc-gohawk -enable=resourcelifetime ./before
# Expected: one finding, exit 3.
GOWORK=off agent-test ../../../.build/golangnyc-gohawk -enable=resourcelifetime ./after
# Expected: no findings, exit 0.
```

After both demos, briefly show the Docker, Caddy, and Kubernetes PRs, then
continue with the origin story. The real-world section does not need another
detailed explanation of concurrency.
