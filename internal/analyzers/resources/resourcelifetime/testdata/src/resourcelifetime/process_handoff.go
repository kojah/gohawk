package resourcelifetime

// Files opened into the elements of a local array and then listed for a
// child process reach os.StartProcess inside the attribute struct. That call
// has no visible body, so an aggregate carrying the resource is an opaque
// boundary: the child inherits the descriptors and nothing here is reported.
// The same array whose elements are only read stays a local leak.

import "os"

func standardStreamsHandedToChild(name string) error {
	files := [3]*os.File{}
	var err error
	files[0], err = os.Open(os.DevNull)
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
