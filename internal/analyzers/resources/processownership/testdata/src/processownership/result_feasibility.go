package processownership

import (
	"os/exec"
	"processdep"
)

// Result guarantees exclude only impossible branches. They neither wait for
// the process nor transfer ownership; every remaining path still owes Wait.
func resultAlwaysNil() error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := processdep.SuccessfulPreparation(); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	return cmd.Wait()
}

func resultAlwaysTrue() error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil {
		return err
	}
	if processdep.Enabled() {
		return cmd.Wait()
	}
	_ = cmd.Process.Kill()
	return nil
}

func locallyDisabled() bool { return false }
func resultAlwaysFalse() error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil {
		return err
	}
	if locallyDisabled() {
		_ = cmd.Process.Kill()
		return nil
	}
	return cmd.Wait()
}

func resultMayFail(failure error) error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil { // want "started command is not waited"
		return err
	}
	if err := processdep.PossibleFailure(failure); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	return cmd.Wait()
}

func resultTypedNilIsError() error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil { // want "started command is not waited"
		return err
	}
	if err := processdep.BoxedNilFailure(); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	return cmd.Wait()
}

func mergedCommandGuaranteedResult() error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil {
		cmd = nil
	}
	if cmd == nil {
		return nil
	}
	if err := processdep.SuccessfulPreparation(); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	return cmd.Wait()
}

// Unresolved dispatch supplies no result contract.
type preparation interface{ Prepare() error }

func resultOpaquePreparation(preparer preparation) error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil { // want "started command is not waited"
		return err
	}
	if err := preparer.Prepare(); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	return cmd.Wait()
}
