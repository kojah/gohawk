package processownership

import "os/exec"

// Helper-created commands may already have another wait owner. This remains
// unknown even when a helper actually constructs an exclusively local command;
// reporting such hidden obligations is an accepted coverage gap.
type commandFactory interface {
	Command() (*exec.Cmd, error)
}

func startInterfaceFactoryCommand(factory commandFactory, wait bool) error {
	cmd, err := factory.Command()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if wait {
		return cmd.Wait()
	}
	return nil
}

func commandWithError() (*exec.Cmd, error) {
	return exec.Command("tool"), nil
}

func startTupleFactoryCommand(wait bool) error {
	cmd, err := commandWithError()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if wait {
		return cmd.Wait()
	}
	return nil
}

func unrelatedTupleFactoryDoesNotOwnCommand(factory commandFactory, wait bool) error {
	_, _ = factory.Command()
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil { // want "started command is not waited"
		return err
	}
	if wait {
		return cmd.Wait()
	}
	return nil
}
