package resourcelifetime

import "os"

func subprocessFile(name string) {
	defer os.Exit(0)
	file, err := os.Create(name)
	if err != nil {
		return
	}
	file.WriteString("child output")
}

func conditionalProcessExit(name string, exit bool) {
	if exit {
		defer os.Exit(0)
	}
	file, err := os.Create(name) // want "owned resource from os.Create is not released on every return path"
	if err != nil {
		return
	}
	file.WriteString("output")
}

func directProcessExit(name string) {
	file, err := os.Create(name)
	if err != nil {
		return
	}
	file.WriteString("output")
	os.Exit(0)
}
