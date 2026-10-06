package resourcelifetime

import "os"

// Destination addresses copied through a local table retain their original
// owner. Unknown destinations are consumption boundaries, not cleanup proofs.
type destinationFiles struct{ first, second *os.File }

func installDestinationFiles(owner *destinationFiles, paths []string) error {
	entries := []struct{ dest **os.File }{{&owner.first}, {&owner.second}}
	for index, entry := range entries {
		file, err := os.Open(paths[index])
		if err != nil {
			return err
		}
		*entry.dest = file
	}
	return nil
}

func loseLocalDestinationFiles(paths []string) error {
	owner := &destinationFiles{}
	entries := []struct{ dest **os.File }{{&owner.first}, {&owner.second}}
	for index, entry := range entries {
		file, err := os.Open(paths[index]) // want "owned resource from os.Open is not released on every return path"
		if err != nil {
			return err
		}
		*entry.dest = file
	}
	return nil
}

func replaceDestinationWithLocal(owner *destinationFiles, path string) error {
	local := &destinationFiles{}
	entry := struct{ dest **os.File }{&owner.first}
	entry.dest = &local.first
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	*entry.dest = file
	return nil
}

func opaqueFileDestination() **os.File

func handToOpaqueDestination(path string) error {
	destination := opaqueFileDestination()
	entry := struct{ dest **os.File }{destination}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	*entry.dest = file
	return nil
}
