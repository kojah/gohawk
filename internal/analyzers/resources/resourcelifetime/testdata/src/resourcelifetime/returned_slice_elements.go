package resourcelifetime

import "os"

func returnedSliceElements(first, second string) ([]*os.File, error) {
	files := make([]*os.File, 2)
	var err error
	files[0], err = os.Open(first)
	if err != nil {
		return nil, err
	}
	files[1], err = os.Open(second)
	if err != nil {
		closeReturnedSliceFiles(files)
		return nil, err
	}
	return files, nil
}

func discardedSliceElements(name string, transfer bool) ([]*os.File, error) {
	files := make([]*os.File, 2)
	file, err := os.Open(name) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return nil, err
	}
	files[0] = file
	if transfer {
		return files, nil
	}
	return nil, nil
}

func unrelatedReturnedSlice(name string, other *os.File) ([]*os.File, error) {
	files := make([]*os.File, 2)
	file, err := os.Open(name) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return nil, err
	}
	files[0] = file
	return []*os.File{other}, nil
}

func closeReturnedSliceFiles(files []*os.File) {
	for _, file := range files {
		if file != nil {
			file.Close()
		}
	}
}
