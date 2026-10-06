package synchelpers

import "sync"

func Waiter(group *sync.WaitGroup) func()        { return func() { group.Wait() } }
func ForwardWaiter(group *sync.WaitGroup) func() { return Waiter(group) }
