package lockorder

import "sync"

// Completion must preserve both callback identity and the mutex argument.
// The contrasting no-op and wrong-receiver cases must not erase a held lock.
func invokeMutexCallback(mutex *sync.Mutex, fn func(*sync.Mutex)) {
	func() { fn(mutex) }()
}

func callbackUnlocksMutex(mutex *sync.Mutex) {
	mutex.Lock()
	invokeMutexCallback(mutex, func(m *sync.Mutex) { m.Unlock() })
}

func callbackIgnoresMutex(mutex *sync.Mutex, early bool) {
	mutex.Lock()
	if early {
		mutex.Unlock()
		return
	}
	invokeMutexCallback(mutex, func(m *sync.Mutex) {})
	return // want "lock .*callbackIgnoresMutex.mutex is not released on this return path"
}

func callbackUnlocksOtherMutex(mutex, other *sync.Mutex, early bool) {
	mutex.Lock()
	if early {
		mutex.Unlock()
		return
	}
	invokeMutexCallback(mutex, func(m *sync.Mutex) { other.Unlock() })
	return // want "lock .*callbackUnlocksOtherMutex.mutex is not released on this return path"
}

func invokeMutexElement(callbacks []func(*sync.Mutex), mutex *sync.Mutex) { callbacks[0](mutex) }

func callbackElementUnlocksMutex(mutex *sync.Mutex) {
	mutex.Lock()
	invokeMutexElement([]func(*sync.Mutex){func(m *sync.Mutex) { m.Unlock() }}, mutex)
}

func callbackElementIgnoresMutex(mutex *sync.Mutex, early bool) {
	mutex.Lock()
	if early {
		mutex.Unlock()
		return
	}
	invokeMutexElement([]func(*sync.Mutex){func(m *sync.Mutex) {}}, mutex)
	return // want "lock .*callbackElementIgnoresMutex.mutex is not released on this return path"
}
