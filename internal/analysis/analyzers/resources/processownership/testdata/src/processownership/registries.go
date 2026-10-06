package processownership

// A started command stored into a map this function does not own, such as a
// package registry keyed by port, belongs to that registry: another function
// looks it up and waits on it. A command stored only into a map the function
// made itself is still owed here.
// https://github.com/alphagov/router/blob/7cfa97b4548fdf02836ab9c01c7df853899af6cd/integration_tests/router_support.go#L119-L127

import "os/exec"

var runningCommands = make(map[int]*exec.Cmd)

func startRegistered(port int) error {
	command := exec.Command("server")
	if err := command.Start(); err != nil {
		return err
	}
	runningCommands[port] = command
	return nil
}

var latestCommand *exec.Cmd

func startLatest() error {
	command := exec.Command("server")
	if err := command.Start(); err != nil {
		return err
	}
	latestCommand = command
	return nil
}

func startIntoLocalMap(port int) error {
	commands := make(map[int]*exec.Cmd)
	command := exec.Command("server")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	commands[port] = command
	return nil
}
