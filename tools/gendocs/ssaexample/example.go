// Package ssaexample holds the function whose SSA form the Understanding SSA
// page prints. It is deliberately small and exercises the lowerings a reader
// meets first: an error-checked call, a branch that merges a variable, a
// closure capturing a local, and a deferred call.
package ssaexample

import (
	"os"
)

// CopyHeader returns the first line of a file, or a default when the file is
// empty. Every lowering it produces is annotated on the documentation page.
func CopyHeader(path string, fallback string) (string, error) {
	file, err := os.Open(path) //nolint:gosec // The path is the example's input, not untrusted data.
	if err != nil {
		return "", err
	}
	defer file.Close() //nolint:errcheck // A deferred close is the lowering the page explains.
	buffer := make([]byte, 64)
	count, err := file.Read(buffer)
	header := fallback
	if err == nil && count > 0 {
		header = string(buffer[:count])
	}
	log := func() { _ = header }
	log()
	return header, nil
}
