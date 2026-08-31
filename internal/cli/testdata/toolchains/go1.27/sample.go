package sample

type identity struct{}

// Generic methods require Go 1.27. This fixture ensures that the analyzer
// process understands the newest supported language version.
func (identity) value[T any](input T) T { return input }

func answer() int { return identity{}.value(42) }
