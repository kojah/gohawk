package processchoices

import "os/exec"

var registeredCommand *exec.Cmd

func registerBeforeStart(cmd *exec.Cmd) { registeredCommand = cmd }

// A void helper can hand the command to an opaque registry. Filtering its
// nonexistent result must not erase the helper's ownership effects.
func voidRegistrationBeforeStart(wait bool) error {
	cmd := exec.Command("tool")
	registerBeforeStart(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	if wait {
		return cmd.Wait()
	}
	return nil
}

func scalarBeforeStart(cmd *exec.Cmd) int { return 1 }

// A scalar result cannot itself own a process. A visible non-retaining
// helper does not transfer the obligation; a conditional Wait still leaves
// the other successful return uncovered.
func scalarResultBeforeStart(wait bool) error {
	cmd := exec.Command("tool")
	_ = scalarBeforeStart(cmd)
	if err := cmd.Start(); err != nil { // want "not waited"
		return err
	}
	if wait {
		return cmd.Wait()
	}
	return nil
}
