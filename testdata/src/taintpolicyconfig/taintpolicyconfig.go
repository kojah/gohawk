package taintpolicyconfig

import (
	"os"
	"os/exec"
)

func configured() {
	_, _ = os.ReadFile(os.Getenv("PATH"))
	_ = exec.Command(os.Getenv("TOOL")) // want "untrusted data reaches process sink exec.Command"
	_ = exec.Command(scrub(os.Getenv("OTHER_TOOL")))
}

func scrub(value string) string { return value }
