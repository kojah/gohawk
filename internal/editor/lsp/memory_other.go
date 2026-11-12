//go:build !linux

package lsp

// processTreeMemory reports that this platform has no measurement. The run
// then relies on the soft GOMEMLIMIT and the timeout alone.
func processTreeMemory(int) (int64, bool) {
	return 0, false
}
