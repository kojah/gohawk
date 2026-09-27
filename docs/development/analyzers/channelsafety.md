# channelsafety design notes

## Retired

The `channelsafety` analyzer and its only check, `send-after-close`, were
removed on 2026-09-27; the last revision containing them is `629d115`.

The check reported a send reachable after a close of the same channel in one
call sequence, including closes and sends inside summarized helpers. It did not
compare events across loop backedges or goroutines. Across every recorded
audit it made one report, a false positive: plow deliberately sends on a
closed channel under a `recover` to capture the panic value. The shape its
proof required panics the first time the code runs, so it rarely survives
into a commit; real send-on-closed-channel bugs are races between a closing
goroutine and a sending one, which the proof excluded.
