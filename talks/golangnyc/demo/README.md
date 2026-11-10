# Live demos: resource leaks and concurrency

Use the file example as a one-minute warm-up, then the Docker and Caddy
examples below as the main demos. Reveal each original PR immediately after
its fix. Allow about eight minutes for these demos and PR reveals together.
Keep HTTP, database, and the original accounts/ledger example as backups.
This separate Go module isolates the intentional defects from the slide examples.

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

## Docker: cleanup exists, but error paths skip it

Open `docker/before/firewall.go`. The function starts a helper, sends firewall
rules, reads output, and waits. A pipe write, close, or read error can return
after `Start` without reaching `Wait`. Closing a pipe is not the same as
collecting the child process's exit status.

```sh
agent-test ../../../.build/golangnyc-gohawk -enable-checks=processownership/missing-wait ./docker/before
agent-test ../../../.build/golangnyc-gohawk -enable-checks=processownership/missing-wait ./docker/after
```

Expect one `processownership/missing-wait` finding (exit 3), then none (exit 0).
This check is experimental and is explicitly enabled here.

Show the diagnostic, then `docker/after/firewall.go`: the fix attaches buffers
and uses `Run` to manage copying and waiting. Reveal
[Docker PR #53517](https://github.com/moby/moby/pull/53517) immediately afterward.
The reduction preserves the pipe operations, early returns, and final wait,
but omits namespace selection, logging, and detailed error messages.

Sources: [before](https://github.com/moby/moby/blob/007054c0ca4ac76866686e4473cfa196731d19d2/daemon/libnetwork/internal/nftables/nft_exec_linux.go),
[submitted fix](https://github.com/moby/moby/blob/4070d0ac379e4b5adde89e41f6d94d202630d960/daemon/libnetwork/internal/nftables/nft_exec_linux.go).

The PR also fixes a pipe deadlock when stderr fills while the parent reads
stdout. That was found during investigation, not diagnosed by this check.
Simply deferring `Wait` would not fix that deadlock; show the `Run` fix.
Analyze these functions without executing them: they name the real `nft`
executable and must not be called against a live firewall.

## Caddy: failure cleanup takes locks in the wrong order

Introduce concurrency, then open `caddy/before/pool.go`.

- Construction publishes a locked entry, releases the pool lock, and runs
  the constructor. A reader needs the pool lock and then the entry lock.
- On failure, cleanup takes the pool lock while still holding the entry lock.
  A reader can hold the pool lock while waiting for the entry. Both can stall.
- Gohawk connects pool-to-entry acquisition during setup with entry-to-pool
  acquisition during cleanup. Use `Range` to explain the concurrent reader.
- Show `caddy/after/pool.go`: release the entry before waiting for the pool.
  A deferred entry unlock at function exit would still happen too late.
- Reveal [Caddy PR #7968](https://github.com/caddyserver/caddy/pull/7968), then
  briefly mention Kubernetes as an additional finding.

```sh
agent-test ../../../.build/golangnyc-gohawk -enable=lockorder ./caddy/before
agent-test ../../../.build/golangnyc-gohawk -enable=lockorder ./caddy/after
```

Expect one `lockorder/contradictory-order` finding (exit 3), then none (exit 0).
This check is core.

This is a single-entry teaching reduction of
[UsagePool.LoadOrNew and Range](https://github.com/caddyserver/caddy/blob/62a72977e58c87fad7e7726c18b58f10c653f2d3/usagepool.go).
Call `Construct` once per Pool; `Range` may run concurrently. The original map,
reference counts, and replacement entries are omitted. The real fix also checks
entry identity before deletion; the single-use slot has no replacement to check.
A closer map-based reduction did not warn with this checkout. This demonstrates
the failure pattern, not a revalidation of the complete current Caddy code.
The reported Caddy combination was available to external modules; built-in
pools did not combine these operations. Analyze the example rather than trying
to reproduce a live hang.

## Backups: other resource types and basic lock ordering

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

For a simpler backup to Caddy, open `concurrency/before/locks.go`.

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

Run the Docker and Caddy before/after commands above before presenting too.
After their PR reveals, briefly show Kubernetes and continue with the origin
story. Do not repeat Docker and Caddy in a separate findings section.
