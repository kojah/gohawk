# Process exit settles only what it reclaims

Decided 2026-09-25. Implemented in `625d618`.

A leak does harm when it accumulates, or when its cleanup has an effect that
program exit would lose: an unflushed writer, an uncommitted transaction, an
orphaned child process. Exit fixes neither, so being in `main` is not an
exemption by itself.

An obligation is settled by process exit only when all of these hold:

1. It arises in `main.main` of package `main`, the entry the Go specification
   defines, not in a function merely named `main`.
2. It runs at most once: outside any loop or closure, in a package that never
   calls or refers to its own `main` (`ssaflow.RunsOnceInProgramEntry`).
3. Every way out of `main` ends the process.
4. The cleanup only reclaims: a descriptor, connection, response body, rows,
   statement, or a goroutine's join. Compressors, transactions, and inferred
   owners keep their obligation.

`resourcelifetime` accepts such a leak; `goroutineownership` marks such a
worker unknown rather than joined, so no analyzer reads exit as proof that the
worker finished. `processownership` is deliberately not covered: not waiting
for a child in `main` may be an intentional launcher or an orphaned worker, and
exit decides neither. Single-caller helpers such as a `runMain` whose result
goes straight to `os.Exit` are a possible extension, not yet covered.
