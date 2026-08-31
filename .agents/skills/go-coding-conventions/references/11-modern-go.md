# §11 Supported-version idioms

- Treat the module `go` directive and supported build matrix as the language
  and API floor. A newer local toolchain does not authorize newer APIs.
- Prefer clearer modern syntax and stdlib packages available at that floor;
  do not rewrite working code merely to showcase a new feature.
- At Go 1.22 or newer, omit obsolete loop-variable capture shadows. Use
  range-over-integer only when it reads more clearly than a conventional loop.
- Use `//go:build` constraints. Retain legacy `// +build` lines only when the
  supported toolchain floor requires them.
- At Go 1.24 or newer, track executable dependencies with `tool` directives in
  `go.mod`; do not add a blank-import `tools.go` workaround.
- Use repository-supported test APIs such as `t.Context`, `b.Loop`, and
  `t.Chdir` when they simplify lifecycle or cleanup. Never raise the Go floor
  accidentally for test convenience.
- Prefer suitable stdlib facilities such as `slices`, `maps`, `cmp`, `iter`,
  and `errors.Join` over hand-rolled equivalents. Use `errors.Join` only when
  callers need every independent error and ordering is not contractual.
- For structured logging, inject `*slog.Logger` into long-lived components and
  derive contextual loggers with `With`. Keep default logger configuration at
  the composition root. Log returned errors once, at an owning boundary.
- Suspect logic, call-site, or package-selection errors before blaming caches.
  Confirm compiled files and dependencies with `go list`; clear caches only
  with evidence of a toolchain or environment fault.
