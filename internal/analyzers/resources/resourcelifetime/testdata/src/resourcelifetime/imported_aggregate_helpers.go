package resourcelifetime

// An imported helper that receives an aggregate holding the resource is
// judged by its summary's contents claim. The claim is loose: a helper that
// stores, sends, starts, or returns anything loaded out of the aggregate
// keeps the call an ownership boundary, and so does one with no summary at
// all. Only a helper proven to keep nothing from inside the aggregate is
// transparent, and then the caller still owes the release.

import (
	"os"

	"resourcedep"
)

func importedHelperKeepsField(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	resourcedep.KeepFirst(&resourcedep.Pair{First: file})
	return nil
}

func importedHelperKeepsFromCopy(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	resourcedep.KeepCopy(&resourcedep.Pair{Second: file})
	return nil
}

func importedHelperPublishesField(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	resourcedep.PublishFirst(&resourcedep.Pair{First: file})
	return nil
}

func importedHelperStartsWithAggregate(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	resourcedep.StartWith(&resourcedep.Pair{First: file})
	return nil
}

func importedHelperReturnsField(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	_ = resourcedep.FirstOf(&resourcedep.Pair{First: file})
	return nil
}

func importedHelperKeepsOnlyName(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	resourcedep.KeepName(&resourcedep.Pair{First: file, Name: path})
	return nil
}

func importedHelperOnlyInspects(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	_ = resourcedep.InspectPair(&resourcedep.Pair{First: file})
	return nil
}

func importedHelperClosesOtherField(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	return resourcedep.CloseSecond(&resourcedep.Pair{First: file})
}
