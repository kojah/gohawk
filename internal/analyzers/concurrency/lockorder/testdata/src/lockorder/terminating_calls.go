package lockorder

// A return reached only through a call that never returns is not a return
// with the lock held: the result summary proves a project's fatal wrapper
// never returns, as os.Exit is documented not to. A wrapper that may return
// keeps the path.

import (
	"os"
	"sync"
)

var terminatingMutex sync.Mutex

func fatal(message string) {
	_ = message
	os.Exit(1)
}

func fatalIf(fail bool) {
	if fail {
		os.Exit(1)
	}
}

func unlockUnlessFatal(fail bool) {
	terminatingMutex.Lock()
	if fail {
		fatal("failed")
		return
	}
	terminatingMutex.Unlock()
}

func unlockUnlessMaybeFatal(fail, other bool) {
	terminatingMutex.Lock()
	if fail {
		fatalIf(other)
		return // want "lock .* is not released on this return path"
	}
	terminatingMutex.Unlock()
}
