// Closure choices provide possible ownership, never a unique target or exact Wait.
package processchoices

import "os/exec"

func register(func() error)
func mixed(flag bool) error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil {
		return err
	}
	other := exec.Command("other")
	var fn func() error
	if flag {
		fn = func() error { return cmd.Wait() }
	} else {
		fn = func() error { return other.Wait() }
	}
	register(fn)
	return nil
}
func launched(flag bool) error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil {
		return err
	}
	other := exec.Command("other")
	var fn func() error
	if flag {
		fn = func() error { return cmd.Wait() }
	} else {
		fn = func() error { return other.Wait() }
	}
	go fn()
	return nil
}
func unrelated(flag bool) error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil { // want "started command is not waited"
		return err
	}
	other := exec.Command("other")
	var fn func() error
	if flag {
		fn = func() error { return other.Wait() }
	} else {
		fn = func() error { return nil }
	}
	register(fn)
	return cmd.Process.Kill()
}
func bypass(flag, skip bool) error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil { // want "started command is not waited"
		return err
	}
	if skip {
		return nil
	}
	var fn func() error
	if flag {
		fn = func() error { return cmd.Wait() }
	} else {
		fn = func() error { return nil }
	}
	register(fn)
	return nil
}

type callback func() error

func converted(flag bool) error {
	cmd := exec.Command("tool")
	if err := cmd.Start(); err != nil { // want "started command is not waited"
		return err
	}
	var fn func() error
	if flag {
		fn = func() error { return cmd.Wait() }
	} else {
		fn = func() error { return nil }
	}
	go callback(fn)()
	return nil
}
