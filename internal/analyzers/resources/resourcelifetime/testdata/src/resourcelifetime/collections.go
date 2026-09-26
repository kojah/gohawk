package resourcelifetime

import (
	"io"
	"os"
)

// Local collections. A resource appended to a slice the function created
// stays owned by the function, through the slice: a range loop that releases
// every element settles it, returning the slice hands it to the caller, and
// a slice dropped without either leaks it. Any other use of the slice, such
// as storing it, passing it on, slicing it, or reading an element outside a
// release loop, leaves the resource unknown, as an append did before.
//
// Gap: an acquisition loop that returns on a later iteration's error does
// not report the resources appended by earlier iterations. The walk cannot
// tell iterations apart, so it reads that error branch as the one where this
// acquisition failed and holds nothing.

func appendedNeverReleased(paths []string) error {
	var files []*os.File
	for _, path := range paths {
		file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
		if err != nil {
			continue
		}
		files = append(files, file)
	}
	return nil
}

func appendedOnceNeverReleased(path string) error {
	var files []*os.File
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	files = append(files, file)
	_ = len(files)
	return nil
}

// A release loop over a different slice does not release this one.
func releasedAnotherSlice(paths []string, others []*os.File) error {
	var files []*os.File
	for _, path := range paths {
		file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
		if err != nil {
			continue
		}
		files = append(files, file)
	}
	for _, other := range others {
		other.Close()
	}
	return nil
}

func rangeReleasesAppended(paths []string) error {
	var files []*os.File
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		files = append(files, file)
	}
	for _, file := range files {
		file.Close()
	}
	return nil
}

func indexedRangeReleasesAppended(paths []string) {
	var files []*os.File
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			files = append(files, file)
		}
	}
	for i := range files {
		files[i].Close()
	}
}

// An interface element is released through the interface's Close.
func rangeReleasesInterfaceElements(paths []string) {
	var closers []io.Closer
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			closers = append(closers, file)
		}
	}
	for _, closer := range closers {
		closer.Close()
	}
}

func appendedOrReleased(path string, keep bool) {
	var files []*os.File
	file, err := os.Open(path)
	if err != nil {
		return
	}
	if keep {
		files = append(files, file)
	} else {
		file.Close()
	}
	for _, kept := range files {
		kept.Close()
	}
}

func collectionReturned(paths []string) ([]*os.File, error) {
	var files []*os.File
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		files = append(files, file)
	}
	return files, nil
}

type openFileSet struct{ files []*os.File }

func collectionStoredOnOwner(set *openFileSet, path string) {
	var files []*os.File
	if file, err := os.Open(path); err == nil {
		files = append(files, file)
	}
	set.files = files
}

func closeEvery(files []*os.File) {
	for _, file := range files {
		file.Close()
	}
}

func collectionPassedToHelper(paths []string) {
	var files []*os.File
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			files = append(files, file)
		}
	}
	closeEvery(files)
}

func collectionDrainedByDefer(paths []string) {
	var files []*os.File
	defer func() {
		for _, file := range files {
			file.Close()
		}
	}()
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			files = append(files, file)
		}
	}
}

// The first element is skipped: not every element is released.
func partialReleaseLoop(paths []string) {
	var files []*os.File
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			files = append(files, file)
		}
	}
	for i := 1; i < len(files); i++ {
		files[i].Close()
	}
}

func conditionalReleaseLoop(paths []string) {
	var files []*os.File
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			files = append(files, file)
		}
	}
	for _, file := range files {
		if file.Name() != "" {
			file.Close()
		}
	}
}

func releaseLoopReturnsEarly(paths []string) error {
	var files []*os.File
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			files = append(files, file)
		}
	}
	for _, file := range files {
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

func elementsOnlyInspected(paths []string) int {
	var files []*os.File
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			files = append(files, file)
		}
	}
	total := 0
	for _, file := range files {
		total += len(file.Name())
	}
	return total
}

func releasedThroughSubslice(paths []string) {
	var files []*os.File
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			files = append(files, file)
		}
	}
	rest := files[0:]
	for _, file := range rest {
		file.Close()
	}
}

func collectionCopied(paths []string, target []*os.File) {
	var files []*os.File
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			files = append(files, file)
		}
	}
	copy(target, files)
}

func collectionSent(paths []string, out chan<- []*os.File) {
	var files []*os.File
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			files = append(files, file)
		}
	}
	out <- files
}

// Appending to the caller's slice can write into the caller's backing array.
func appendedToCallerSlice(files []*os.File, path string) {
	if file, err := os.Open(path); err == nil {
		files = append(files, file)
		_ = files
	}
}
