# §7 Concurrency

- Default to synchronous library APIs. Callers choose concurrency; never hide
  goroutines or return channels as an API without an owned lifecycle.
- Every goroutine needs an exit condition and join owner. Use caller context,
  `errgroup.Group`, or `sync.WaitGroup`.
- Use channels for handoff and signaling; use mutexes for shared state.
- Use channel capacity zero or one unless a comment justifies another bound.
- Put `ctx context.Context` first. Never store it or pass `nil`.
- Prefer `sync.Once` or atomics over hand-rolled flag-plus-mutex protocols.

Before concurrent code, explicitly decide failure propagation, cancellation,
error collection, output ordering, concurrency bounds, and who waits for every
started goroutine. Repository policy may add package-specific retry and test
rules.
