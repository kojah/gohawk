package resourcelifetime

// A visible helper that releases the elements of the aggregate it receives
// inside a loop is an uncertainty boundary: the every-return proof cannot
// credit a call inside a cycle, and which element an iteration releases is
// decided by iteration. The caller is not reported. A helper whose cleanup
// depends on a flag, or that loops over some other collection, keeps the
// obligation open and is reported. An imported helper carries the same loop
// as a may-claim in its summary, so a variadic close helper from another
// package is the same boundary.

import (
	"os"

	"resourcedep"
)

func closeEachGuarded(files [2]*os.File) error {
	for _, file := range files {
		if file != nil {
			_ = file.Close()
		}
	}
	return nil
}

func closeEachInSlice(files []*os.File) {
	for _, file := range files {
		_ = file.Close()
	}
}

func closeEachThroughPointer(files *[2]*os.File) {
	for _, file := range files {
		if file != nil {
			_ = file.Close()
		}
	}
}

type fileSet struct{ files []*os.File }

func (set *fileSet) closeAll() {
	for _, file := range set.files {
		_ = file.Close()
	}
}

func arrayReleasedByLoopingHelper(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return closeEachGuarded([2]*os.File{file, nil})
}

func sliceReleasedByLoopingHelper(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	closeEachInSlice([]*os.File{file})
	return nil
}

func arrayPointerReleasedByLoopingHelper(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	files := [2]*os.File{file, nil}
	closeEachThroughPointer(&files)
	return nil
}

func setReleasedByLoopingMethod(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	set := &fileSet{files: []*os.File{file}}
	set.closeAll()
	return nil
}

func closeWhenFlagged(file *os.File, flagged bool) {
	if flagged {
		_ = file.Close()
	}
}

func flaggedHelperKeepsObligation(path string, flagged bool) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	closeWhenFlagged(file, flagged)
	return nil
}

func closeOthers(files [2]*os.File, others []*os.File) {
	_ = files
	for _, other := range others {
		_ = other.Close()
	}
}

func loopOverOtherCollectionKeepsObligation(path string, others []*os.File) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	closeOthers([2]*os.File{file, nil}, others)
	return nil
}

func inspectEach(files [2]*os.File) {
	for _, file := range files {
		_ = file
	}
}

func loopWithoutCleanupKeepsObligation(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	inspectEach([2]*os.File{file, nil})
	return nil
}

func variadicReleasedByImportedHelper(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer resourcedep.CloseEach("temp", file)
	return nil
}

func variadicInspectedByImportedHelperKeepsObligation(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	resourcedep.InspectEach(file)
	return nil
}
