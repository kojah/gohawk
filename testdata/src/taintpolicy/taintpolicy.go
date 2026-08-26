package taintpolicy

import (
	"fmt"
	"os"
	"os/exec"
)

func badPath() {
	path := os.Getenv("INPUT_PATH")
	_, _ = os.ReadFile(path) // want "untrusted data reaches filesystem sink os.ReadFile"
}

func badCommand() {
	name := os.Getenv("TOOL")
	_ = exec.Command(name) // want "untrusted data reaches process sink exec.Command"
}

func badTerminal() {
	value := os.Getenv("VALUE")
	_, _ = fmt.Fprintln(os.Stdout, value) // want "untrusted data reaches terminal sink fmt.Fprintln"
}

func goodPath() {
	path := validatePath(os.Getenv("INPUT_PATH"))
	_, _ = os.ReadFile(path)
}

func validatePath(value string) string { return value }
