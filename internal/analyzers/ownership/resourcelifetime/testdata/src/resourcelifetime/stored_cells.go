package resourcelifetime

import "os"

// Stored cells: a resource is settled through the storage holding it rather
// than through the value the acquisition produced. Both forms below are proven
// releases, not suppressions -- the trace records release-proven for each, and
// the second reaches it through same-access-path. Nothing else pins that, so a
// change to the dominating-store or projection-stability rules would otherwise
// break them silently.

type storedCellWrapper struct {
	file *os.File
}

// Two different resources reach one variable on different branches. The target
// is defined where it is stored, so every path on which it is live passes its
// own store and the deferred close settles both.
func storedCellFromBranches(useFirst bool) error {
	var file *os.File
	var err error
	if useFirst {
		file, err = os.Open("first")
	} else {
		file, err = os.Open("second")
	}
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

// The resource is closed through a field of a local aggregate rather than
// through the value os.Open returned. The field path still names the storage
// the resource was written to, so the release is exact.
func storedCellThroughField() error {
	file, err := os.Open("input")
	if err != nil {
		return err
	}
	holder := &storedCellWrapper{}
	holder.file = file
	defer holder.file.Close()
	return nil
}

// A later store to the same field replaces what the deferred close releases, so
// the first resource is the one left open.
func storedCellReplacedField() error {
	first, err := os.Open("first") // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	holder := &storedCellWrapper{}
	holder.file = first
	second, err := os.Open("second")
	if err != nil {
		return err
	}
	holder.file = second
	defer holder.file.Close()
	return nil
}
