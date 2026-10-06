package ifaceuse

import (
	"ifacedep"
	"sync"
)

type quiet struct{ n int }

func (q *quiet) Do() { q.n++ }

func quietUnderLock(mu *sync.Mutex) {
	ifacedep.WithLock(mu, &quiet{})
}

func forwardInterface(mu *sync.Mutex, d ifacedep.Doer) {
	ifacedep.WithLock(mu, d)
}
