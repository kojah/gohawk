package resourcelifetime

import "os"

func withFileCallback(file *os.File, fn func(*os.File)) {
	func() { fn(file) }()
}

func callbackClosesFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	withFileCallback(file, func(f *os.File) { f.Close() })
}

func callbackIgnoresFile(path string) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	withFileCallback(file, func(f *os.File) {})
}

func callbackClosesOtherFile(path string, other *os.File) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	withFileCallback(file, func(f *os.File) { other.Close() })
}
