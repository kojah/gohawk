package lockorder

import (
	"sync"
	"time"
)

// A callback handed to another owner makes subsequent held-lock state unknown.
// This does not prove when, or even whether, the callback runs.
func callbackTimerUnlock() {
	var mu sync.Mutex
	mu.Lock()
	time.AfterFunc(time.Second, mu.Unlock)
	mu.Lock()
	mu.Unlock()
}

func callbackFieldUnlock(owner *struct{ ready func() }) {
	var mu sync.Mutex
	mu.Lock()
	owner.ready = mu.Unlock
	mu.Lock()
}

func callbackLiteralUnlock(owner *struct{ ready func() }) {
	var mu sync.Mutex
	mu.Lock()
	owner.ready = func() { mu.Unlock() }
	mu.Lock()
	mu.Unlock()
}

func callbackOtherMutex(owner *struct{ ready func() }, mu, other *sync.Mutex) {
	mu.Lock()
	owner.ready = other.Unlock
	mu.Lock() // want "lock `mu` is acquired while already held"
	mu.Unlock()
}

func callbackNotHandedOff(mu *sync.Mutex) {
	mu.Lock()
	callback := mu.Unlock
	_ = callback
	mu.Lock() // want "lock `mu` is acquired while already held"
	mu.Unlock()
}
