package forwarder

import (
	"dependency"
	"sync"
)

func Pair(b, a *sync.Mutex) { dependency.Pair(a, b) }
