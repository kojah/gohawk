package resourcelifetime

import (
	"os"
	"resourcedep"
)

// A resource may pass through a view returned by one helper before a second
// helper publishes the view. The view has no Close method, but publication
// makes local abandonment unprovable. Mere observation still leaks it.
func publishedWrappedWriter(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	resourcedep.PublishWriter(resourcedep.WrapWriter(file))
	return nil
}

func inspectedWrappedWriter(path string) error {
	file, err := os.Create(path) // want "owned resource from os.Create is not released on every return path"
	if err != nil {
		return err
	}
	resourcedep.InspectWriter(resourcedep.WrapWriter(file))
	return nil
}
