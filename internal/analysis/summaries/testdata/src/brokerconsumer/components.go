package brokerconsumer

import (
	"brokerdependency"
	"brokerforward"
	"sync"
)

func Close(resource *brokerdependency.Resource) { brokerforward.Close(resource) }
func Lock(mu *sync.Mutex)                       { brokerforward.Lock(mu) }
func Result() error                             { return brokerforward.Result() }

func Pair(first, second *sync.Mutex)      { brokerforward.Pair(first, second) }
func LocalPair(first, second *sync.Mutex) { localPair(first, second) }
func localPair(first, second *sync.Mutex) {
	first.Lock()
	second.Lock()
	second.Unlock()
	first.Unlock()
}
func Opaque(mu *sync.Mutex) { brokerdependency.Unknown(mu) }
