package sample

func identity[T any](value T) T { return value }

func answer() int { return identity(42) }
