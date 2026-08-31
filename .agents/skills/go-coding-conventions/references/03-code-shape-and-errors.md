# §3 Code shape

- Keep happy path at left margin. Return early on errors.
- Omit `else` after a terminating branch.
- Declare variables in smallest useful scope. Prefer scoped error checks when
  error does not outlive branch.
- Split functions that need internal section comments.

```go
func (s *Store) Get(id string) (*Item, error) {
	if id == "" {
		return nil, ErrEmptyID
	}
	item, ok := s.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}
```

# §4 Errors

- Handle each error once: recover locally or return it; never log and return.
- Wrap with `%w`. Describe attempted operation in lowercase with no trailing
  punctuation or `failed to` prefix.

```go
return fmt.Errorf("decoding request: %w", err)
```

- Inspect wrapped errors with `errors.Is` and `errors.As`; never string-match.
- Use sentinel errors for expected classes; use structured error types when
  callers need fields.
- Never panic on expected failure. Recover only at owned goroutine boundaries
  and re-report panic.
