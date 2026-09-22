package brokerdependency

import "sync"

type Resource struct{}

func (*Resource) Close()       {}
func Close(resource *Resource) { resource.Close() }
func Lock(mu *sync.Mutex)      { mu.Lock(); mu.Unlock() }
func Result() error            { return nil }
