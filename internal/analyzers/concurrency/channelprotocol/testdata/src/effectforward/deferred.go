package effectforward

import (
	"effecthelpers"
	"sync"
)

func DeferredWorker(mu *sync.Mutex, done chan struct{}) { effecthelpers.DeferredWorker(mu, done) }
func PreludeWorker(mu, other *sync.Mutex, done chan struct{}) {
	effecthelpers.PreludeWorker(mu, other, done)
}
