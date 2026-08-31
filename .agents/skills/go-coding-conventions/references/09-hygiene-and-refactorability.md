# §9 Hygiene

- Prefer nil slices unless wire format distinguishes `[]` from `null`.
  Preallocate when size is known.
- Ban mutable global state. Inject anything tests would replace, including
  clocks.
- Delete dead and commented-out code.
- Use `any`, not `interface{}`. Use generics only for real duplication.
- Wrap third-party SDKs behind owned adapter interfaces; domain packages must
  not import SDKs directly.

# §10 Refactorability check

A change forcing unrelated packages to change indicates a broken seam. Verify:

1. Dependencies flow one way.
2. Interfaces live consumer-side.
3. Constructors receive dependencies explicitly.
4. Tests assert behavior, not structure.
5. Exported surface remains minimal.
