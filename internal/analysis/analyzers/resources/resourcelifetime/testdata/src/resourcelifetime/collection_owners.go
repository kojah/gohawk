package resourcelifetime

import "os"

// Collection-selected owners are an uncertainty boundary. This deliberately
// misses leaks when a purely local collection of owners is itself discarded;
// proving that all collection elements stay local is outside this classifier.

type cachedFileOwner struct{ file *os.File }

func retainedCollectionOwner(owners map[int]*cachedFileOwner, path string) error {
	var selected []*cachedFileOwner
	for _, owner := range owners {
		selected = append(selected, owner)
	}
	for _, owner := range selected {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		owner.file = file
	}
	return nil
}

func localOwnerDiscarded(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	owner := &cachedFileOwner{file: file}
	_ = owner
	return nil
}
