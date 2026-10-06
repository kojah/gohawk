package resourcelifetime

import "os"

// A generic helper's instantiation is a wrapper that calls the generic body.
// When the body returns its argument, closing the result closes the
// acquired resource, whether directly or from a deferred literal.

func mustValue[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func firstOf[T any](value, other T) T {
	_ = value
	return other
}

// Accepted: a deferred literal closes the file mustValue returned.
func genericMustClosedByLiteral(path string) {
	file := mustValue(os.Create(path))
	defer func() { _ = file.Close() }()
	_, _ = file.WriteString("x")
}

// The result is never closed.
func genericMustNeverClosed(path string) {
	file := mustValue(os.Create(path)) // want "owned resource from os.Create is not released"
	_, _ = file.WriteString("x")
}

// firstOf returns its other argument, so closing its result does not close
// the created file.
func genericHelperReturnsOther(path string, fallback *os.File) {
	created, err := os.Create(path) // want "owned resource from os.Create is not released"
	if err != nil {
		return
	}
	chosen := firstOf(created, fallback)
	defer func() { _ = chosen.Close() }()
}
