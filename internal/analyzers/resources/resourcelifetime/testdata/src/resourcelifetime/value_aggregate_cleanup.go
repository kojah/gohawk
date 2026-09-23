package resourcelifetime

// A resource stored in a struct that is passed by value reaches the helper
// through the compiler's spill of the parameter. The completion search and the
// exported summary both see a Close on the spilled field as a Close of the
// parameter, so the helper settles the caller's obligation whether it is a
// function, a value-receiver method, or lives in another package. A helper
// that only reads the field leaves the obligation open. Arrays passed by
// value, alone or inside a struct, follow the same spill.
//
// Accepted gap: a discharge summary is a fact about the whole parameter, so a
// helper that closes a different element or field of the copy still settles
// the caller, exactly as it does for a pointer parameter today.

import (
	"os"

	"resourcedep"
)

type valueJob struct{ out *os.File }

func finishValueJob(j valueJob) error { return j.out.Close() }

func (j valueJob) finish() error { return j.out.Close() }

func inspectValueJob(j valueJob) error {
	_ = j.out
	return nil
}

func valueJobClosedByHelper(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return finishValueJob(valueJob{out: file})
}

func valueJobClosedByValueMethod(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	j := valueJob{out: file}
	return j.finish()
}

func valueJobClosedAcrossPackages(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return resourcedep.CloseHolder(resourcedep.FileHolder{File: file})
}

func valueJobLeakedByHelper(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return err
	}
	return inspectValueJob(valueJob{out: file})
}

func finishFileArray(files [2]*os.File) error { return files[0].Close() }

func fileArrayClosedByHelper(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return finishFileArray([2]*os.File{file, nil})
}

type fileBatch struct{ files [2]*os.File }

func finishFileBatch(b fileBatch) error { return b.files[0].Close() }

func fileBatchClosedByHelper(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return finishFileBatch(fileBatch{files: [2]*os.File{file, nil}})
}

func inspectFileArray(files [2]*os.File) error {
	_ = files[0]
	return nil
}

func fileArrayLeakedByHelper(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return err
	}
	return inspectFileArray([2]*os.File{file, nil})
}
