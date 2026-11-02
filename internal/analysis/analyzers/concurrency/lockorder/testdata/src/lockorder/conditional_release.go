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

// A saved result keeps the call it came from. The edge proof settles only
// the acquisition that call could have seen held: a lock reacquired after
// the helper returned keeps its own obligation on this return.
// https://github.com/kojah/gohawk/pull/58#discussion_r4437363025

func (c *conditionalGate) runReacquired() {
	c.mu.Lock()
	released := c.rejectIfInactive()
	if released {
		c.mu.Lock()
		if released {
			return // want "lock `.*` is not released on this return path"
		}
	}
	c.mu.Unlock()
}

// The ordering bound also holds inside one block: an acquisition made after
// the tested call cannot be the one its edge release discharged.
func (c *conditionalGate) runAcquiredAfter() {
	released := c.rejectIfInactive()
	c.mu.Lock()
	if released {
		return // want "lock `.*` is not released on this return path"
	}
	c.mu.Unlock()
}

// A retry loop reacquires and calls the helper again before testing its
// result, so the tested call always sees the current acquisition held and
// its release applies. The call dominates the reacquire, which is why the
// ordering bound asks whether the branch is reachable from the acquisition
// without re-executing the call, not whether the call comes first.
func (c *conditionalGate) runRetry() {
	c.mu.Lock()
	for {
		if c.rejectIfInactive() {
			return
		}
		c.mu.Unlock()
		c.mu.Lock()
	}
}
