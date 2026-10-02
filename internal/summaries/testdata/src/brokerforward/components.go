package brokerforward

import (
	"brokerdependency"
	"sync"
)

func Close(resource *brokerdependency.Resource) { brokerdependency.Close(resource) }
func Lock(mu *sync.Mutex)                       { brokerdependency.Lock(mu) }
func Result() error                             { return brokerdependency.Result() }

func Pair(first, second *sync.Mutex) { brokerdependency.Pair(first, second) }
