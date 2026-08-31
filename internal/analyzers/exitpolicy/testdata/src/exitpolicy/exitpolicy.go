package exitpolicy

import (
	"context"
	"log"
	"os"
)

func cleanup() {}

func fatalAfterDefer() {
	defer cleanup()
	log.Fatal("failed") // want "log.Fatal exits without running an earlier defer"
}

func fatalAfterDeferredCancel() {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	log.Fatal("failed")
}

func exitAfterConditionalDefer(clean bool) {
	if clean {
		defer cleanup()
	}
	os.Exit(1) // want "os.Exit exits without running an earlier defer"
}

func exitBeforeDefer(fail bool) {
	if fail {
		os.Exit(1)
	}
	defer cleanup()
}

func panicRunsDefers() {
	defer cleanup()
	panic("failed")
}
