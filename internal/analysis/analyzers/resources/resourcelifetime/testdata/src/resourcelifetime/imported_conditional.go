package resourcelifetime

import (
	"os"
	"resourceforward"
)

func importedConditionalCleanup(path string, ready bool) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	if resourceforward.CloseWhenReady(file, ready) {
		return
	}
	file.Close()
}

func importedConditionalWrongFile(path string, other *os.File, ready bool) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	if resourceforward.CloseWhenReady(other, ready) {
		return
	}
	file.Close()
}
