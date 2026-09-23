package resourcelifetime

// Files opened into the elements of a local array and then listed for a
// child process reach os.StartProcess inside the attribute struct. The
// child inherits the descriptors, but the parent's *os.File is still the
// parent's to close: os.StartProcess is summarized as reading each file's
// descriptor and keeping none of the files, which is what os/exec relies on
// when it closes the parent's copies after Start. Both forms are reported.

import "os"

func standardStreamsHandedToChild(name string) error {
	files := [3]*os.File{}
	var err error
	files[0], err = os.Open(os.DevNull) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	files[1] = os.Stdout
	files[2] = os.Stderr
	attr := &os.ProcAttr{Files: []*os.File{files[0], files[1], files[2]}}
	process, err := os.StartProcess(name, []string{name}, attr)
	if err != nil {
		return err
	}
	return process.Release()
}

func streamsOnlyInspected(name string) error {
	files := [3]*os.File{}
	var err error
	files[0], err = os.Open(os.DevNull) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	_ = []*os.File{files[0]}
	return nil
}
