package resourcelifetime

import (
	"os"
	"resourcedep"
	"sync"
)

// A registry's retention of a separate aggregate owner is unknown, not proof
// of cleanup. Unclosed retained owners are a deliberate coverage gap; passing
// an aggregate to a helper that merely observes it is not a retention boundary.
type aggregateRegistry struct{ entries sync.Map }
type registryFile struct{ file *os.File }

func (r *aggregateRegistry) keep(owner *registryFile) { r.entries.Store(1, owner) }
func (r *aggregateRegistry) close() {
	r.entries.Range(func(key, value any) bool {
		_ = value.(*registryFile).file.Close()
		r.entries.Delete(key)
		return true
	})
}

func registryAggregateCleanup(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	r := &aggregateRegistry{}
	r.keep(&registryFile{file: file})
	r.close()
	return nil
}

func inspectRegistryFile(owner *registryFile) bool { return owner.file != nil }

func registryAggregateObservedOnly(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	_ = inspectRegistryFile(&registryFile{file: file})
	return nil
}

func importedRegistryRetainsVariadicFile(registry *resourcedep.ValueRegistry, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	registry.Keep(file, nil)
	return nil
}

func importedRegistryOnlyObservesVariadicFile(registry *resourcedep.ValueRegistry, path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	registry.Observe(file, nil)
	return nil
}
