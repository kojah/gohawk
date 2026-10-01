package resourcelifetime

import (
	"io"
	"os"
	"resourcedep"
)

func importedAsyncWriter() error {
	file, err := os.Create("fixture")
	if err != nil {
		return err
	}
	resourcedep.StartWriter(file)
	return nil
}

func importedSynchronousWriter() error {
	file, err := os.Create("fixture") // want "owned resource from os.Create is not released on every return path"
	if err != nil {
		return err
	}
	resourcedep.InspectWriter(file)
	return nil
}

func importedOtherAsyncWriter(other io.Writer) error {
	file, err := os.Create("fixture") // want "owned resource from os.Create is not released on every return path"
	if err != nil {
		return err
	}
	resourcedep.StartOtherWriter(file, other)
	return nil
}

func importedAsyncWriterBypassed(skip bool) error {
	file, err := os.Create("fixture") // want "owned resource from os.Create is not released on every return path"
	if err != nil {
		return err
	}
	if skip {
		return nil
	}
	resourcedep.StartWriter(file)
	return nil
}
