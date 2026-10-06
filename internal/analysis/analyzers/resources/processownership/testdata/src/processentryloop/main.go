package main

import "os/exec"

var stop bool

func main() {
	for {
		command := exec.Command("child")
		if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
			return
		}
		_ = command.Process.Kill()
		if stop {
			return
		}
	}
}
