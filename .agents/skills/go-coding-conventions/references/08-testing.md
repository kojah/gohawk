# §8 Testing

Design for testing through explicit interfaces, contexts, and injected
dependencies.

- Default to table-driven tests through exported APIs and black-box
  `package foo_test`.
- Use `t.Fatal` for setup failures and `t.Error` for assertions. Include input,
  got, and want in failure messages. Mark helpers with `t.Helper()`.
- Prefer small fakes over call-sequence mocks.
- Use stdlib `testing` plus `go-cmp`; avoid assertion frameworks.
- Put complex fixtures and contract-worthy golden output under `testdata/`.
  Normalize nondeterministic fields and require an explicit update mode.
- Use `t.Parallel()` only for independent tests; never blanket-add it to
  subtests.
- Prefer `t.Cleanup()`, injected clocks, or `testing/synctest`; never coordinate
  with `time.Sleep`.
