package resourcelifetime

import (
	"os"
	"resourceforward"
)

func importedReturnedCleanup(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer resourceforward.CleanupFor(file)()
}

func importedReturnedWrongCleanup(path string, other *os.File) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	defer resourceforward.CleanupFor(other)()
	_ = file.Name()
}

func importedReturnedCleanupTooLate(path string, early bool) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	if early {
		return
	}
	defer resourceforward.CleanupFor(file)()
}
