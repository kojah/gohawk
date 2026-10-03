package resourcelifetime

import (
	"os"
	"resourcedep"
)

// A returned owner may hold a value-copy wrapper through a local cell.
// The shared heap summaries preserve its untouched writer field.
func returnedValueCopy(path string) (any, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	writer := resourcedep.NewValueWriter(file).WithLevel(1).Context().Extract()
	return &struct{ Writer *resourcedep.ValueWriter }{Writer: &writer}, nil
}

func discardedValueCopy(path string) error {
	file, err := os.Create(path) // want "owned resource from os.Create is not released"
	if err != nil {
		return err
	}
	_ = resourcedep.NewValueWriter(file).WithLevel(1).Context().Extract()
	return nil
}

func returnedReplacementCopy(path string) (any, error) {
	file, err := os.Create(path) // want "owned resource from os.Create is not released"
	if err != nil {
		return nil, err
	}
	writer := resourcedep.NewValueWriter(file).WithOutput(os.Stderr).Context().Extract()
	return &struct{ Writer *resourcedep.ValueWriter }{Writer: &writer}, nil
}

func returnedConditionalValueCopy(path string, hook func()) (any, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	writer := resourcedep.NewValueWriter(file).WithLevel(1).Context().WithHook(hook).Extract()
	return &struct{ Writer *resourcedep.ValueWriter }{Writer: &writer}, nil
}

func discardedConditionalValueCopy(path string, hook func()) error {
	file, err := os.Create(path) // want "owned resource from os.Create is not released"
	if err != nil {
		return err
	}
	_ = resourcedep.NewValueWriter(file).WithLevel(1).Context().WithHook(hook).Extract()
	return nil
}

func returnedConditionalReplacementCopy(path string, hook func()) (any, error) {
	file, err := os.Create(path) // want "owned resource from os.Create is not released"
	if err != nil {
		return nil, err
	}
	writer := resourcedep.NewValueWriter(file).WithOutput(os.Stderr).Context().WithHook(hook).Extract()
	return &struct{ Writer *resourcedep.ValueWriter }{Writer: &writer}, nil
}
