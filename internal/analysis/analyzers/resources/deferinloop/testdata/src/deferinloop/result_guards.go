package deferinloop

// An exported helper's proven result prunes the branch it can never take: a
// loop that always returns after its first deferred cleanup never reaches
// the backedge, so the defer does not outlive an iteration. A condition the
// helper leaves open keeps the backedge reachable.

import (
	"os"

	"iteratorowner"
)

func deferBeforeCertainReturn(paths []string) {
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		defer file.Close()
		if iteratorowner.AlwaysDone() {
			return
		}
	}
}

func deferBeforeUncertainReturn(paths []string, done bool) {
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		defer file.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
		if done {
			return
		}
	}
}
