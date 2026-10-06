package main

import "os/exec"

// A one-time entry command has program-lifetime ownership uncertainty; killing
// it does not prove Wait. Repeated and reusable launches stay in adjacent controls.
func main() {
	command := exec.Command("child")
	if err := command.Start(); err != nil {
		return
	}
	_ = command.Process.Kill()
}
