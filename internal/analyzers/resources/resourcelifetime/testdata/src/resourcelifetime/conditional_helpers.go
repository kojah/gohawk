package resourcelifetime

import "os"

func closeFileWhenReady(file *os.File, ready bool) bool {
	if !ready {
		return false
	}
	file.Close()
	return true
}

func forwardConditionalClose(file *os.File, ready bool) bool {
	return closeFileWhenReady(file, ready)
}

func conditionalFileCleanup(path string, ready bool) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	if forwardConditionalClose(file, ready) {
		return
	}
	file.Close()
}

func conditionalCleanupTooLate(path string, ready, early bool) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	if early {
		return
	}
	if closeFileWhenReady(file, ready) {
		return
	}
	file.Close()
}
