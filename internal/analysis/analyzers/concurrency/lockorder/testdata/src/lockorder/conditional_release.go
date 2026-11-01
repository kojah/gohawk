package lockorder

import (
	"errors"
	"sync"
)

// A helper may release a caller-held lock only on the return path whose
// outcome the caller's branch selects. The releasing arm is settled inside
// the helper; the held arm keeps its own obligation.
// https://github.com/kojah/gohawk/issues/57

var errRejected = errors.New("rejected")

type conditionalGate struct {
	mu     sync.Mutex
	active bool
}

func (c *conditionalGate) rejectIfInactive() bool {
	if !c.active {
		c.mu.Unlock()
		return true
	}
	return false
}

func (c *conditionalGate) run() error {
	c.mu.Lock()
	if c.rejectIfInactive() {
		return errRejected
	}
	c.mu.Unlock()
	return nil
}

func (c *conditionalGate) runMulti() error {
	c.mu.Lock()
	if rejected, _ := c.rejectErr(); rejected {
		return errRejected
	}
	c.mu.Unlock()
	return nil
}

func (c *conditionalGate) rejectErr() (bool, error) {
	if !c.active {
		c.mu.Unlock()
		return true, errRejected
	}
	return false, nil
}

var condGateMu sync.Mutex

func rejectGlobal(active bool) bool {
	if !active {
		condGateMu.Unlock()
		return true
	}
	return false
}

// A helper that reaches the lock only through a package global is outside the
// argument-mapping evidence the edge proof uses; the release stays unproven
// and the diagnostic remains.
func runGlobal(active bool) error {
	condGateMu.Lock()
	if rejectGlobal(active) {
		return errRejected // want "lock `.*` is not released on this return path"
	}
	condGateMu.Unlock()
	return nil
}

// Only the arm whose outcome carries the release is settled. A result that
// says nothing usable about the arm, an arm that never releases, or a caller
// that drops the unreleased path keeps the diagnostic.

func (c *conditionalGate) runLeaky() {
	c.mu.Lock()
	if c.rejectIfInactive() {
		return
	}
	return // want "lock `.*` is not released on this return path"
}

func (c *conditionalGate) rejectWithoutResult() bool {
	if !c.active {
		c.mu.Unlock()
	}
	return c.active
}

func (c *conditionalGate) runConfused() {
	c.mu.Lock()
	if c.rejectWithoutResult() {
		return // want "lock `.*` is not released on this return path"
	}
	c.mu.Unlock()
	return
}

func (c *conditionalGate) rejectSometimes() bool {
	if !c.active {
		c.mu.Unlock()
	}
	return true
}

func (c *conditionalGate) runSometimes() {
	c.mu.Lock()
	if c.rejectSometimes() {
		if c.active {
			c.mu.Unlock()
		}
		return // want "lock `.*` is not released on this return path"
	}
	c.mu.Unlock()
}
