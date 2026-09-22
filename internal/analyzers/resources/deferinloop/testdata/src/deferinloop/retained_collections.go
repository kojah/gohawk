package deferinloop

import "os"

// Collection ownership may deliberately extend the resource past its iteration.
// The loop-local check does not decide the lifetime of a containing aggregate.
func retainedFilesBeforeDefer(names []string) error {
	var files []*os.File
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		files = append(files, file)
		defer file.Close()
	}
	for _, file := range files {
		_, _ = file.Stat()
	}
	return nil
}

func retainedFilesAfterDefer(names []string) error {
	var files []struct{ file *os.File }
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		files = append(files, struct{ file *os.File }{file})
	}
	for _, entry := range files {
		_, _ = entry.file.Stat()
	}
	return nil
}

func unrelatedCollection(names []string) error {
	var copies []string
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		copies = append(copies, name)
		defer file.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
	}
	return nil
}
