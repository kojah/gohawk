# §5 Interfaces

- Accept narrow interfaces and return concrete structs.
- Write concrete behavior first. Add an interface when a consumer needs
  substitution; do not predict future implementations.
- Define interfaces in consumer packages; implementations satisfy them
  implicitly.
- Keep interfaces to one to three methods; compose small interfaces.

```go
type Store interface {
	Get(ctx context.Context, id string) (*Item, error)
	Put(ctx context.Context, item *Item) error
}
```

# §6 Structs and constructors

- Prefer useful zero values. When invariants make that impossible, provide
  `NewX` and make invalid zero-value behavior explicit and predictable.
- Use functional options only for many independent optional knobs with useful
  defaults. Use a config struct for required values. Avoid long positional
  parameter lists and options that permit invalid intermediate state.
- Use pointer receivers for mutation, large structs, or receiver consistency.
  Never mix pointer and value receivers on one type.
- Embed only for genuine composition; named fields otherwise.
- Group files by feature, not type kind. Avoid `models.go` dumping grounds.
