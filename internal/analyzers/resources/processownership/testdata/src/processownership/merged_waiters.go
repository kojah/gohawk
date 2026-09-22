package processownership

import "os/exec"

// An immediate successful-Start merge preserves the exact selected receiver.
// Other possible identities remain uncertainty, not proof of reaping.
// Accepted coverage gap: a merge that can choose the wrong command can hide a
// real missing Wait; direct unrelated receivers and early bypasses still report.
func fallbackCommandWait() error {
	command := exec.Command("first")
	if err := command.Start(); err != nil {
		command = exec.Command("fallback")
		if err := command.Start(); err != nil {
			return err
		}
	}
	return command.Wait()
}

func unrelatedCommandWait(other *exec.Cmd) error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	_ = command.Process.Kill()
	return other.Wait()
}

func bypassMergedWait(other *exec.Cmd, chooseOther, skip bool) error {
	command := exec.Command("tool")
	if err := command.Start(); err != nil { // want "started command is not waited on every successful return path"
		return err
	}
	if skip {
		return nil
	}
	selected := command
	if chooseOther {
		selected = other
	}
	return selected.Wait()
}
