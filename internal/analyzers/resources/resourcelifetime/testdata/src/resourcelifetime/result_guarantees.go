package resourcelifetime

import (
	"os"
	"resourceforward"
)

func cleanupAfterAlwaysNil(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	if resourceforward.NilResult() != nil {
		return
	}
	file.Close()
}

func cleanupAfterTypedNil(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	if resourceforward.TypedNilResult() == nil {
		return
	}
	file.Close()
}

func cleanupAfterTrue(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	if resourceforward.TrueResult() {
		file.Close()
	}
}

func cleanupAfterFalse(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	if resourceforward.FalseResult() {
		return
	}
	file.Close()
}

func typedNilStillLeaks(path string) {
	file, err := os.Open(path) // want "owned resource .* is not released"
	if err != nil {
		return
	}
	if resourceforward.TypedNilResult() != nil {
		return
	}
	file.Close()
}

func mixedResultStillLeaks(path string, yes bool) {
	file, err := os.Open(path) // want "owned resource .* is not released"
	if err != nil {
		return
	}
	if resourceforward.MixedResult(yes) != nil {
		return
	}
	file.Close()
}

func mutableResultStillLeaks(path string) {
	file, err := os.Open(path) // want "owned resource .* is not released"
	if err != nil {
		return
	}
	if resourceforward.MutableResult() != nil {
		return
	}
	file.Close()
}

func deferredResultStillLeaks(path string) {
	file, err := os.Open(path) // want "owned resource .* is not released"
	if err != nil {
		return
	}
	if resourceforward.DeferredResult() != nil {
		return
	}
	file.Close()
}
