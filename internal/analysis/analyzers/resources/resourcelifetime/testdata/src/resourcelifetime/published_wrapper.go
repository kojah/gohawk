package resourcelifetime

import (
	"io"
	"os"
	"resourcedep"
)

// A resource may pass through a view returned by one helper before a second
// helper publishes the view. The view has no Close method, but publication
// makes local abandonment unprovable. Mere observation still leaks it.
// Discarded local slices of wrapped resources remain an accepted false-negative
// gap: their append can become opaque consumption before publication is queried.
func publishedWrappedWriter(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	resourcedep.PublishWriter(resourcedep.WrapWriter(file))
	return nil
}

type writerRegistry struct {
	writers []io.Writer
	options []resourcedep.WriterOption
}

func appendedPublishedWrapper(path string, registry *writerRegistry) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	registry.writers = append(registry.writers, resourcedep.WrapWriter(file))
	return nil
}

func appendedConfiguredWrapper(path string, registry *writerRegistry, console bool) error {
	var file *os.File
	if console {
		file = os.Stdout
	} else {
		opened, err := os.Create(path)
		if err != nil {
			return err
		}
		file = opened
	}
	registry.options = append(registry.options, resourcedep.ConfigureWriter(resourcedep.WrapWriter(file)))
	return nil
}

func appendedUnrelatedWrapper(path string, registry *writerRegistry) error {
	file, err := os.Create(path) // want "owned resource from os.Create is not released on every return path"
	if err != nil {
		return err
	}
	registry.writers = append(registry.writers, resourcedep.WrapWriter(os.Stdout))
	_ = file
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
