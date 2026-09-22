package condsafety

import (
	"condhelper"
	"sync"
)

func await(condition *sync.Cond) { condition.Wait() }

func localHelper() {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	await(condition) // want "Cond.Wait called with its mutex unlocked"
}

func importedHelper() {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	condhelper.Acquire(&mutex)
	condhelper.Release(&mutex)
	condhelper.Await(condition) // want "Cond.Wait called with its mutex unlocked"
}

func acquiredInHelper() {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	condhelper.Acquire(&mutex)
	condhelper.Await(condition)
}

func otherMutex() {
	var first, second sync.Mutex
	condition := sync.NewCond(&first)
	second.Lock()
	condition.Wait() // want "Cond.Wait called with its mutex unlocked"
}

func released() {
	var mutex sync.Mutex
	condition := sync.NewCond(&mutex)
	mutex.Lock()
	mutex.Unlock()
	condition.Wait() // want "Cond.Wait called with its mutex unlocked"
}

func embeddedMutexReleased() {
	var owner struct{ mutex sync.Mutex }
	condition := sync.NewCond(&owner.mutex)
	condhelper.Acquire(&owner.mutex)
	condhelper.Release(&owner.mutex)
	condition.Wait() // want "Cond.Wait called with its mutex unlocked"
}

func embeddedMutexHeld() {
	var owner struct{ mutex sync.Mutex }
	condition := sync.NewCond(&owner.mutex)
	condhelper.Acquire(&owner.mutex)
	defer condhelper.Release(&owner.mutex)
	condition.Wait()
}
