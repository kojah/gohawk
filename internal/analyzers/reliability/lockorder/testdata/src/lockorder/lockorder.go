package lockorder

import (
	"errors"
	"sync"
)

var regressionFirst sync.Mutex
var regressionSecond sync.Mutex
var readLock sync.RWMutex

type localMutexOwner struct {
	mutex sync.Mutex
}

func distinctLocalMutexOwners() {
	first := &localMutexOwner{}
	second := &localMutexOwner{}
	first.mutex.Lock()
	defer first.mutex.Unlock()
	second.mutex.Lock()
	defer second.mutex.Unlock()
}

func regressionForward() {
	regressionFirst.Lock()
	defer regressionFirst.Unlock()
	regressionSecond.Lock()
	defer regressionSecond.Unlock()
}

func regressionReverse() {
	regressionSecond.Lock()
	defer regressionSecond.Unlock()
	regressionFirst.Lock() // want "contradictory lock order: regressionFirst and regressionSecond"
	defer regressionFirst.Unlock()
}

func missingUnlock(skip bool) {
	regressionFirst.Lock()
	if skip {
		return // want "lock regressionFirst is not released on this return path"
	}
	regressionFirst.Unlock()
}

func missingReadUnlock(skip bool) {
	readLock.RLock()
	if skip {
		return // want "lock readLock is not released on this return path"
	}
	readLock.RUnlock()
}

func deferredUnlock(skip bool) {
	first.Lock()
	defer first.Unlock()
	if skip {
		return
	}
}

func deferredCallbackUnlock() {
	regressionFirst.Lock()
	release := func() { regressionFirst.Unlock() }
	defer release()
}

func deferredOnceCallbackUnlock() {
	regressionFirst.Lock()
	release := sync.OnceFunc(func() { regressionFirst.Unlock() })
	defer release()
}

func nestedDeferredOnceCallbackUnlock() {
	regressionFirst.Lock()
	release := sync.OnceFunc(func() { regressionFirst.Unlock() })
	defer func() { release() }()
}

func discardUnlockCallback(func()) func() { return func() {} }

func discardedDeferredCallback(skip bool) {
	regressionFirst.Lock()
	defer discardUnlockCallback(func() { regressionFirst.Unlock() })()
	if skip {
		return // want "lock regressionFirst is not released on this return path"
	}
	regressionFirst.Unlock()
}

func wrongDeferredCallback(skip bool) {
	regressionFirst.Lock()
	release := func() { regressionSecond.Unlock() }
	defer release()
	if skip {
		return // want "lock regressionFirst is not released on this return path"
	}
	regressionFirst.Unlock()
}

func deferredGuardEstablishedAfterLock(skip bool) {
	locked := false
	defer func() {
		if locked {
			regressionFirst.Unlock()
		}
	}()
	regressionFirst.Lock()
	locked = true
	if skip {
		return
	}
}

func earlierDeferUnlocksDifferentLock(skip bool) {
	locked := false
	defer func() {
		if locked {
			regressionSecond.Unlock()
		}
	}()
	regressionFirst.Lock()
	locked = true
	if skip {
		return // want "lock regressionFirst is not released on this return path"
	}
	regressionFirst.Unlock()
}

func conditionallyRegisteredEarlierDefer(install, skip bool) {
	locked := false
	if install {
		defer func() {
			if locked {
				regressionFirst.Unlock()
			}
		}()
	}
	regressionFirst.Lock()
	locked = true
	if skip {
		return // want "lock regressionFirst is not released on this return path"
	}
	regressionFirst.Unlock()
}

func deferredUnlockInLoop(lock *sync.Mutex, values []int) {
	for range values {
		lock.Lock() // want "lock lockorder.deferredUnlockInLoop.lock is acquired while already held"
		defer lock.Unlock()
	}
}

// Returning with a lock held on every path may intentionally transfer the
// critical section to the caller, so the analyzer stays quiet without evidence
// of a local release policy.
func intentionallyHeld() {
	regressionFirst.Lock()
}

// A trailing false result marks a return as unsuccessful, so a claim helper
// that releases before returning false and holds on returning true is
// acquiring for its caller.
func claimOrRelease(claimed bool) (int, bool) {
	regressionFirst.Lock()
	if claimed {
		return 1, true
	}
	regressionFirst.Unlock()
	return 0, false
}

// The convention does not excuse a successful return that forgot the unlock
// when another successful return released it.
func claimForgetsUnlock(claimed, other bool) (int, bool) {
	regressionFirst.Lock()
	if claimed {
		return 1, true // want "lock .*regressionFirst is not released on this return path"
	}
	if other {
		regressionFirst.Unlock()
		return 2, true
	}
	regressionFirst.Unlock()
	return 0, false
}

func conditionallyAcquired(index, other int) {
	if index != other {
		regressionFirst.Lock()
	}
	if index != other {
		regressionFirst.Unlock()
	}
}

func conditionallyAcquiredByBareBool(lock bool) {
	if lock {
		regressionFirst.Lock()
	}
	if lock {
		regressionFirst.Unlock()
	}
}

func conditionallyAcquiredByDifferentBools(acquire, release bool) {
	if acquire {
		regressionFirst.Lock() // want "lock regressionFirst is not released on this return path"
	}
	if release {
		regressionFirst.Unlock()
	}
}

func conditionallyAcquiredByOppositeGuard(lock bool) {
	if lock {
		regressionFirst.Lock() // want "lock regressionFirst is not released on this return path"
	}
	if !lock {
		regressionFirst.Unlock()
	}
}

func conditionalNil(pointer *int) {
	if pointer != nil {
		regressionFirst.Lock()
	}
	if pointer != nil {
		regressionFirst.Unlock()
	}
}

func conditionFromComputed(hashLock int) {
	refLock := computedLock(hashLock)
	if hashLock != refLock {
		regressionFirst.Lock()
	}
	if hashLock != refLock {
		regressionFirst.Unlock()
	}
}

func computedLock(value int) int { return value + 1 }

func transferredUnlock() {
	var mutex sync.Mutex
	mutex.Lock()
	done := make(chan struct{})
	go func() {
		mutex.Unlock()
		close(done)
	}()
	mutex.Lock()
	mutex.Unlock()
	<-done
}

type branchingUnlockOwner struct {
	mutex sync.Mutex
	ready bool
}

func (owner *branchingUnlockOwner) unlockOnEveryReturn() {
	if !owner.ready {
		owner.mutex.Unlock()
		return
	}
	owner.ready = false
	owner.mutex.Unlock()
}

func transferredBranchingUnlock(owner *branchingUnlockOwner) {
	owner.mutex.Lock()
	go owner.unlockOnEveryReturn()
}

func calledBranchingUnlock(owner *branchingUnlockOwner) {
	owner.mutex.Lock()
	owner.unlockOnEveryReturn()
}

func calledClosureUnlock(fail, notify bool) {
	var mutex sync.Mutex
	mutex.Lock()
	release := func() {
		if notify {
			computedLock(1)
		}
		mutex.Unlock()
	}
	if fail {
		release()
		return
	}
	mutex.Unlock()
}

type calledClosureOwner struct {
	mutex sync.Mutex
	errCh chan error
}

func calledClosureFieldUnlock(owner *calledClosureOwner, fail bool) {
	owner.mutex.Lock()
	release := func(err error) {
		if owner.errCh != nil {
			owner.errCh <- err
		}
		owner.mutex.Unlock()
	}
	if fail {
		release(errors.New("failed"))
		return
	}
	owner.mutex.Unlock()
}

func calledClosureUnlocksDifferentMutex(fail bool) {
	var mutex sync.Mutex
	var other sync.Mutex
	mutex.Lock()
	release := func() { other.Unlock() }
	if fail {
		release()
		return // want "lock lockorder.calledClosureUnlocksDifferentMutex:local:mutex:t0 is not released on this return path"
	}
	mutex.Unlock()
}

func calledClosureConditionallyUnlocks(fail, release bool) {
	var mutex sync.Mutex
	mutex.Lock()
	conditionalRelease := func() {
		if release {
			mutex.Unlock()
		}
	}
	if fail {
		conditionalRelease()
		return // want "lock lockorder.calledClosureConditionallyUnlocks:local:mutex:t1 is not released on this return path"
	}
	mutex.Unlock()
}

func repeatedTransferredUnlock() {
	var mutex sync.Mutex
	mutex.Lock()
	done := make(chan struct{})
	go func() {
		for {
			mutex.Unlock()
			mutex.Lock()
			select {
			case done <- struct{}{}:
				return
			default:
			}
		}
	}()
	for range 10 {
		mutex.Lock()
		mutex.Unlock()
	}
	<-done
}

func nestedRepeatedTransferredUnlock() {
	func() {
		mutex := sync.Mutex{}
		mutex.Lock()
		done := make(chan struct{})
		go func() {
			for {
				mutex.Unlock()
				mutex.Lock()
				select {
				case done <- struct{}{}:
					return
				default:
				}
			}
		}()
		for range 10 {
			mutex.Lock()
			mutex.Unlock()
		}
		<-done
	}()
}

func conditionalGoroutineUnlock(release, fail bool) {
	var mutex sync.Mutex
	mutex.Lock()
	go func() {
		if release {
			mutex.Unlock()
		}
	}()
	if fail {
		return // want "lock lockorder.conditionalGoroutineUnlock:local:mutex:t1 is not released on this return path"
	}
	mutex.Unlock()
}

type guardedState struct {
	mutex sync.Mutex
}

type pendingCall struct {
	mutex sync.Mutex
}

func lockDynamicCalls(ids []string, calls map[string]*pendingCall, fail bool) error {
	prepared := make([]*pendingCall, 0, len(ids))
	for _, id := range ids {
		pending := calls[id]
		pending.mutex.Lock()
		if fail {
			for _, item := range prepared {
				item.mutex.Unlock()
			}
			pending.mutex.Unlock()
			return errors.New("failed")
		}
		prepared = append(prepared, pending)
	}
	for _, item := range prepared {
		item.mutex.Unlock()
	}
	return nil
}

func lockLoopSelectedMutex(mutexes []*sync.Mutex) {
	var selected *sync.Mutex
	for _, candidate := range mutexes {
		selected = candidate
	}
	if selected == nil {
		return
	}
	selected.Lock()
	selected.Unlock()
}

var firstState guardedState
var secondState guardedState

var interfaceFirst sync.Mutex
var interfaceSecond sync.Mutex

func nestedFieldForward() {
	firstState.mutex.Lock()
	defer firstState.mutex.Unlock()
	secondState.mutex.Lock()
	defer secondState.mutex.Unlock()
}

func nestedFieldReverse() {
	secondState.mutex.Lock()
	defer secondState.mutex.Unlock()
	firstState.mutex.Lock() // want "contradictory lock order: firstState.mutex and secondState.mutex"
	defer firstState.mutex.Unlock()
}

func localFieldMissingUnlock(skip bool) {
	state := new(guardedState)
	state.mutex.Lock()
	if skip {
		return // want "lock lockorder.localFieldMissingUnlock:local:new:t0.mutex is not released on this return path"
	}
	state.mutex.Unlock()
}

func distinctDynamicFieldLocks(first, second *guardedState) {
	first.mutex.Lock()
	defer first.mutex.Unlock()
	second.mutex.Lock()
	defer second.mutex.Unlock()
}

func conditionalDeferredUnlock(fail bool) {
	var mutex sync.Mutex
	unlocked := false
	mutex.Lock()
	defer func() {
		if !unlocked {
			mutex.Unlock()
		}
	}()
	if fail {
		return
	}
	mutex.Unlock()
	unlocked = true
}

func temporarilyReleaseBorrowed(state *guardedState) {
	state.mutex.Unlock()
	computedLock(1)
	state.mutex.Lock()
}

func returnedUnlockOwner() func() {
	var mutex sync.Mutex
	mutex.Lock()
	return sync.OnceFunc(func() { mutex.Unlock() })
}

func returnedUnlockOwnerWithError(fail bool) (release func(), err error) {
	var mutex sync.Mutex
	mutex.Lock()
	release = sync.OnceFunc(func() { mutex.Unlock() })
	defer func() {
		if err != nil {
			release()
		}
	}()
	if fail {
		return nil, errors.New("failed")
	}
	return release, nil
}

type returnedLockOwner struct {
	mutex sync.RWMutex
}

func (owner *returnedLockOwner) returnedFieldUnlockWithError(fail bool) (release func(), err error) {
	owner.mutex.RLock()
	release = sync.OnceFunc(func() { owner.mutex.RUnlock() })
	defer func() {
		if err != nil {
			release()
		}
	}()
	if fail {
		return nil, errors.New("failed")
	}
	return release, nil
}

func (owner *returnedLockOwner) returnedLocalFieldUnlockWithError(fail bool) (_ func(), err error) {
	owner.mutex.RLock()
	release := sync.OnceFunc(func() { owner.mutex.RUnlock() })
	defer func() {
		if err != nil {
			release()
		}
	}()
	if fail {
		return nil, errors.New("failed")
	}
	return release, nil
}

func (owner *returnedLockOwner) returnedLoopFieldUnlockWithError(ready func() bool, fail bool) (_ func(), err error) {
	for {
		owner.mutex.RLock()
		if ready() {
			break
		}
		owner.mutex.RUnlock()
	}
	release := sync.OnceFunc(func() { owner.mutex.RUnlock() })
	defer func() {
		if err != nil {
			release()
		}
	}()
	if fail {
		return nil, errors.New("failed")
	}
	return release, nil
}

func lockEveryStripe(locks []sync.Mutex) {
	for index := range locks {
		locks[index].Lock()
	}
}

func interfaceForward() {
	var first sync.Locker = &interfaceFirst
	var second sync.Locker = &interfaceSecond
	first.Lock()
	defer first.Unlock()
	second.Lock()
	defer second.Unlock()
}

func interfaceReverse() {
	var first sync.Locker = &interfaceFirst
	var second sync.Locker = &interfaceSecond
	second.Lock()
	defer second.Unlock()
	first.Lock() // want "contradictory lock order: interfaceFirst and interfaceSecond"
	defer first.Unlock()
}

func unknownInterfaceLock(lock sync.Locker) {
	lock.Lock()
	defer lock.Unlock()
}

func knownInterfaceMissingUnlock(skip bool) {
	var lock sync.Locker = &interfaceFirst
	lock.Lock()
	if skip {
		return // want "lock interfaceFirst is not released on this return path"
	}
	lock.Unlock()
}

func ambiguousInterfaceLock(reverse, skip bool) {
	var lock sync.Locker
	if reverse {
		lock = &interfaceFirst
	} else {
		lock = &interfaceSecond
	}
	lock.Lock()
	if skip {
		return
	}
	lock.Unlock()
}

type tryLocker interface {
	sync.Locker
	TryLock() bool
}

func convertedInterfaceMissingUnlock(skip bool) {
	var extended tryLocker = &interfaceFirst
	var lock sync.Locker = extended
	lock.Lock()
	if skip {
		return // want "lock interfaceFirst is not released on this return path"
	}
	lock.Unlock()
}

func sameOriginInterfaceLock(branch, skip bool) {
	var lock sync.Locker
	if branch {
		lock = &interfaceFirst
	} else {
		lock = &interfaceFirst
	}
	lock.Lock()
	if skip {
		return // want "lock interfaceFirst is not released on this return path"
	}
	lock.Unlock()
}

type capture struct {
	mu   sync.Mutex
	seen int
}

// Two captured owners of the same type are distinct locks inside a closure.
func closureLocksDistinctCaptures(capA, capB *capture, run func(func())) {
	run(func() {
		capA.mu.Lock()
		capB.mu.Lock()
		capA.seen++
		capB.seen++
		capA.mu.Unlock()
		capB.mu.Unlock()
	})
}

func closureLocksSameCaptureTwice(capA *capture, run func(func())) {
	run(func() {
		capA.mu.Lock()
		capA.mu.Lock() // want "lock lockorder.closureLocksSameCaptureTwice\\$1:free:capA.mu is acquired while already held"
		capA.mu.Unlock()
		capA.mu.Unlock()
	})
}

type operationClient struct {
	operationMu sync.Mutex
	oidcLock    *sync.Mutex
	reload      func() error
}

// beginOperation returns with operationMu held on every successful return and
// releases it on every error return; endOperation is its counterpart.
func (c *operationClient) beginOperation() error {
	c.operationMu.Lock()
	if c.oidcLock == nil {
		return nil
	}
	c.oidcLock.Lock()
	if c.reload != nil {
		if err := c.reload(); err != nil {
			c.oidcLock.Unlock()
			c.operationMu.Unlock()
			return err
		}
	}
	return nil
}

func (c *operationClient) endOperation() {
	if c.oidcLock != nil {
		c.oidcLock.Unlock()
	}
	c.operationMu.Unlock()
}

// A successful return that forgets the unlock while another successful return
// releases is not an acquisition contract.
func (c *operationClient) forgetsUnlockOnOnePath(skip bool) error {
	c.operationMu.Lock()
	if skip {
		return nil // want "lock .*operationMu is not released on this return path"
	}
	c.operationMu.Unlock()
	return nil
}

// Each iteration locks a different mutex, so the repeated acquisition through
// the same instruction is not recursive.
func locksEveryKeyMutex(keys map[string]*sync.Mutex, entries []*sync.Mutex) {
	for _, m := range keys {
		m.Lock()
		defer m.Unlock()
	}
	for _, m := range entries {
		m.Lock()
		defer m.Unlock()
	}
}

type loopedLocker struct{ mu sync.Mutex }

// The same receiver field locked on every iteration is recursive.
func (l *loopedLocker) locksSameMutexInLoop(items []int) {
	for range items {
		l.mu.Lock() // want "lock .*mu is acquired while already held"
	}
	l.mu.Unlock()
}

type sessionState struct{ mu sync.Mutex }

type sessionManager struct {
	states map[string]*sessionState
}

func (m *sessionManager) state(id string) *sessionState { return m.states[id] }

// A state looked up by the iteration key is a different mutex each time.
func (m *sessionManager) locksEverySessionState(ids []string) {
	for _, id := range ids {
		state := m.state(id)
		state.mu.Lock()
		defer state.mu.Unlock()
	}
}

// A lookup with a fixed key returns the same state, so locking it on every
// iteration is recursive.
func (m *sessionManager) locksFixedStateInLoop(ids []string) {
	for range ids {
		state := m.state("fixed")
		state.mu.Lock() // want "lock .*mu is acquired while already held"
	}
}

type releaser interface{ Release(*guardedConn) }

type guardedConn struct {
	mu       sync.Mutex
	releaser releaser
}

// The connection is handed to an interface method while locked; that method
// may unlock it, so the return afterwards is unknown rather than a defect.
func (c *guardedConn) handOffLocked(early bool) {
	c.mu.Lock()
	if early {
		c.mu.Unlock()
		return
	}
	c.releaser.Release(c)
}

// An unrelated value handed off does not settle the lock.
func (c *guardedConn) handOffOther(other *guardedConn, early bool) {
	c.mu.Lock() // want "lock .*mu is not released on this return path"
	if early {
		c.mu.Unlock()
		return
	}
	c.releaser.Release(other)
}
