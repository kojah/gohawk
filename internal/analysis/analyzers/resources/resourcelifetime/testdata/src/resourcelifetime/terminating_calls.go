package resourcelifetime

// A return reached only through a call that never returns is not a return
// that leaks: the result summary proves a project's fatal wrapper never
// returns, as os.Exit is documented not to, so the path ends there. A
// wrapper that may return keeps the early return reachable.

import "os"

func fatal(message string) {
	_ = message
	os.Exit(1)
}

func fatalIf(fail bool) {
	if fail {
		os.Exit(1)
	}
}

func closedUnlessFatal(path string, fail bool) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	if fail {
		fatal("failed")
		return nil
	}
	return file.Close()
}

func closedUnlessMaybeFatal(path string, fail, other bool) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	if fail {
		fatalIf(other)
		return nil
	}
	return file.Close()
}
