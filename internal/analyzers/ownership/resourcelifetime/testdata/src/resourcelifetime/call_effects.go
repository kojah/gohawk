package resourcelifetime

import "os"

// Observing a resource cannot discharge its obligation. The same observation
// of its holder also cannot erase the field's identity before cleanup.
func inspectEffectFile(file *os.File) bool         { return file != nil }
func inspectEffectOwner(owner *snapshotOwner) bool { return owner.file != nil }

func readOnlyDoesNotDischarge(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	inspectEffectFile(file)
	return nil
}

func closeAfterReadOnlyOwner(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	owner := snapshotOwner{file: file}
	inspectEffectOwner(&owner)
	return owner.file.Close()
}
