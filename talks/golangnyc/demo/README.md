# Opening live demo: one file leak

Allow about two minutes: show the code, run Gohawk, explain the diagnostic,
then fix the leak and run it again. Both versions fit on one screen.
This is a separate Go module so the other intentional talk defects do not
appear in the opening demo.

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

After the demo: “I’ve also run this on projects like Docker, Caddy, and
Kubernetes and found real bugs. I’ll show you some of those in a moment—but
first, let me tell you what made me want to build this.”
