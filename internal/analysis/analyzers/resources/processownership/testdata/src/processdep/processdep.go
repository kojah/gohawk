package processdep

import "os/exec"

func Wait(command *exec.Cmd) error { return command.Wait() }

func MaybeWait(command *exec.Cmd, enabled bool) error {
	if enabled {
		return command.Wait()
	}
	return nil
}

// InvokeWithPanicRecovery has a recovery return in SSA even though the panic
// path rethrows. A must-invoke summary can conservatively remain unavailable.
func InvokeWithPanicRecovery(fn func() error) {
	defer func() {
		if value := recover(); value != nil {
			panic(value)
		}
	}()
	_ = fn()
}
