package condhelper

import "sync"

func Await(condition *sync.Cond) { condition.Wait() }
func Release(mutex *sync.Mutex)  { mutex.Unlock() }
func Acquire(mutex *sync.Mutex)  { mutex.Lock() }
