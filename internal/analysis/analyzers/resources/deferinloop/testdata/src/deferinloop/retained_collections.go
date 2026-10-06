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

// A nested collection does not retain a resource on its zero-iteration path.
// Empty/comment-only YAML inputs take exactly this path in Sloth:
// https://github.com/slok/sloth/blob/8a3be4fab79defa4448d09d91b48422615980b05/cmd/sloth/commands/generate.go#L200-L213
func conditionallyRetainedOutputs(names []string, records [][]string) error {
	var outputs []struct {
		file *os.File
		data string
	}
	for index, name := range names {
		file, err := os.Create(name)
		if err != nil {
			return err
		}
		defer file.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
		for _, record := range records[index] {
			outputs = append(outputs, struct {
				file *os.File
				data string
			}{file, record})
		}
	}
	for _, output := range outputs {
		_, _ = output.file.WriteString(output.data)
	}
	return nil
}
