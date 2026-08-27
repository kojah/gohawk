package docresourcelifetime

import "os"

//gohawk:example flagged
func read(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	_ = file
	return nil
}

//gohawk:example end

//gohawk:example ok
func readSafely(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

//gohawk:example end
