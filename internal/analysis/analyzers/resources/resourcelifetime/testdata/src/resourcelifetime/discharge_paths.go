package resourcelifetime

// A helper's cleanup claim names the path it cleans up beneath its
// parameter, so a caller is credited only for the resource it stored at that
// path, whether the helper is imported or visible. A visible helper that
// closes the other field, or the other element, leaves the obligation open;
// before paths, any resource the argument contained was credited. The
// mismatching imported forms are beside the kept-contents claim that makes
// them reportable, in imported_aggregate_helpers.go.

import (
	"os"

	"resourcedep"
)

func closedThroughMatchingField(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return resourcedep.CloseFirst(&resourcedep.Pair{First: file})
}

// ClosePairSecond closes only the second file of a pair.
func ClosePairSecond(pair *resourcedep.Pair) error { return pair.Second.Close() }

func closedThroughOtherField(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	return ClosePairSecond(&resourcedep.Pair{First: file})
}

func closedThroughMatchingElement(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return resourcedep.CloseHead([2]*os.File{file, nil})
}

// CloseTailOf closes only the second element.
func CloseTailOf(files [2]*os.File) error { return files[1].Close() }

func closedThroughOtherElement(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	return CloseTailOf([2]*os.File{file, nil})
}
