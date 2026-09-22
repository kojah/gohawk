package effectforward

import (
	"effecthelpers"
	"sync"
)

func Worker(done chan struct{}, mu *sync.Mutex) { effecthelpers.Worker(mu, done) }
