package processownership

import (
	"os"
	"os/exec"
)

// A returned process-handle owner is an uncertain wait handoff. Keeping only
// its PID, discarding the owner, or keeping another process does not qualify.
func returnedHandleOwner() (*processFieldOwner, error) {
	command := exec.Command("tool")
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &processFieldOwner{process: command.Process}, nil
}

func returnedHandleSlice() ([]*os.Process, error) {
	command := exec.Command("tool")
	if err := command.Start(); err != nil {
		return nil, err
	}
	return []*os.Process{command.Process}, nil
}

func returnedDifferentHandleOwner(other *os.Process, wait bool) (*processFieldOwner, error) {
	command := exec.Command("tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return nil, err
	}
	if wait {
		command.Wait()
	}
	_ = command.Process.Pid
	return &processFieldOwner{process: other}, nil
}

func returnedProcessPID(wait bool) (int, error) {
	command := exec.Command("tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return 0, err
	}
	if wait {
		command.Wait()
	}
	return command.Process.Pid, nil
}

func conditionalReturnedHandleOwner(transfer bool) (*processFieldOwner, error) {
	command := exec.Command("tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return nil, err
	}
	if transfer {
		return &processFieldOwner{process: command.Process}, nil
	}
	return nil, nil
}
