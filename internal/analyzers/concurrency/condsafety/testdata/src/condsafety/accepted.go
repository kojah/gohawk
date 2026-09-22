package condsafety

import "sync"

// No absence-of-Lock inference for borrowed mutexes, opaque lockers, escaped
// conditions, changed L fields, or concurrently accessed mutexes.
func borrowed(mutex *sync.Mutex)        { sync.NewCond(mutex).Wait() }
func borrowedCond(condition *sync.Cond) { condition.Wait() }
func custom(locker sync.Locker)         { sync.NewCond(locker).Wait() }

func held() {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	mutex.Lock()
	condition.Wait()
}

func otherGoroutine() {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	go mutex.Lock()
	condition.Wait()
}

func escaped(callback func(*sync.Cond)) {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	callback(condition)
	condition.Wait()
}

func changedLocker(other *sync.Mutex) {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	condition.L = other
	condition.Wait()
}

func rwmutex() {
	var mutex sync.RWMutex
	condition := sync.NewCond(mutex.RLocker())
	mutex.RLock()
	condition.Wait()
}

func cannotReachWait() {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	mutex.Unlock()
	condition.Wait()
}

func signalsNeedNotHoldLock() {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	condition.Signal()
	condition.Broadcast()
}

type lookalike struct{}

func conditionalLock(lock bool) {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	if lock { mutex.Lock() }
	condition.Wait()
}

func doubleLockBeforeWait() {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	mutex.Lock()
	mutex.Lock()
	condition.Wait()
}

func helperConstructor(mutex *sync.Mutex) *sync.Cond { return sync.NewCond(mutex) }
func opaqueConstruction() {
	var mutex sync.Mutex
	helperConstructor(&mutex).Wait()
}

func copiedOwner() {
	var owner struct{ mutex sync.Mutex }
	condition := sync.NewCond(&owner.mutex)
	owner.mutex.Lock()
	owner = struct{ mutex sync.Mutex }{}
	condition.Wait()
}

func (*lookalike) Wait() {}
func misleading()        { var condition lookalike; condition.Wait() }
