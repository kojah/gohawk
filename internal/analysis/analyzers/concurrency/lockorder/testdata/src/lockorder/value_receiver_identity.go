package lockorder

import "sync"

type valueReceiverLockContext struct {
	valuesLock sync.Mutex
}

type valueReceiverAction struct {
	*valueReceiverLockContext
}

func exerciseValueReceiver(action valueReceiverAction, kind int, fail bool) {
	action.releasedAfterSwitch(kind, fail)
	action.missingAfterSwitch(kind, fail)
}

// Each promoted field access creates a separate SSA Field, sometimes after
// reloading the by-value receiver. A stable receiver still names one mutex.
func (action valueReceiverAction) releasedAfterSwitch(kind int, fail bool) {
	stable := action
	switch kind {
	case 0:
		stable.valuesLock.Lock()
		stable.valuesLock.Unlock()
	case 1:
		stable.valuesLock.Lock()
		stable.valuesLock.Unlock()
	}
	if fail {
		return
	}
}

func (action valueReceiverAction) missingAfterSwitch(kind int, fail bool) {
	stable := action
	switch kind {
	case 0:
		stable.valuesLock.Lock()
	case 1:
		stable.valuesLock.Lock()
	default:
		return
	}
	if fail {
		return // want "lock `stable\\.valuesLock` is not released on this return path"
	}
	stable.valuesLock.Unlock()
}

// The receiver parameter remains part of the identity: two actions are not
// collapsed merely because they project the same embedded field declaration.
func distinctValueReceiverActions(first, second valueReceiverAction) {
	first.valuesLock.Lock()
	second.valuesLock.Lock()
	second.valuesLock.Unlock()
	first.valuesLock.Unlock()
}
