# §1 Naming

- Use MixedCaps, never underscores. Scale name length with scope.
- Keep acronym case consistent: `userID`, `HTTPClient`, `parseURL`.
- Avoid package-name stutter: `chubby.File`, not `chubby.ChubbyFile`.
- Drop `Get` from getters; retain `Set` for setters.
- End one-method interfaces in `-er` when natural.
- Use consistent one- or two-letter receiver names; never `self` or `this`.
- Name error variables `ErrNotFound`; name error types `NotFoundError`.
- Give domain states, closed sets, identifiers, and bitsets defined types when
  doing so makes invalid interchange harder or operations clearer. Pair enums
  and flags with named constants, and put repeated bitset operations behind
  small methods. Keep counts, indexes, offsets, capacities, and other genuinely
  numeric values primitive unless distinct units would otherwise be confused.
- Use `type State uint8`, not `type State = uint8`, for semantic separation.
  Type aliases preserve identity and are primarily for compatibility or
  re-exporting an existing type.

# §2 Packages

- Name packages for what they provide: short, lowercase, singular. Ban
  grab-bag `util`, `common`, and `base` packages.
- Add new code to the closest cohesive package by default. Split a package only
  when the new boundary owns distinct invariants or vocabulary, serves multiple
  consumers, isolates a platform or third-party dependency, or enforces
  dependency direction.
- Prefer several focused files in one package over one package per type,
  operation, workflow phase, or source file. `internal/` controls visibility;
  it does not justify another package.
- Treat one-production-file or one-consumer packages as review signals, not
  automatic defects. Merge neighboring packages when most exported API only
  shuttles the same values across their boundary or neither side can state an
  independent contract.
- Put packages without external consumers under `internal/`. Export only for a
  real external consumer.
- Resolve import cycles by merging packages or extracting shared lower-level
  types.
- Never perform I/O, read environment variables, or start goroutines in
  `init()`. Permit only cheap deterministic setup.
- Keep command packages limited to parsing and wiring. Put logic in
  `internal/`; dependencies flow from composition roots toward leaf packages.
  Domain logic must not import transport or storage adapters.
