package grouphelper

import "sync"

func Finish(group *sync.WaitGroup) { group.Done() }
