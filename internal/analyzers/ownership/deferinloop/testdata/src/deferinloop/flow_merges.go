package deferinloop

import "os"

// These cases keep statuses separate at joins. An unknown or settled path
// must neither create nor erase the evidence from another live path.
func cleanupOnBothBranches(names []string, branch bool) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		if branch {
			_ = file.Close()
		} else {
			closeFile(file)
		}
	}
	return nil
}

func opaqueOrSettled(names []string, branch bool, consume func(*os.File)) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		if branch {
			consume(file)
		} else {
			_ = file.Close()
		}
	}
	return nil
}

func opaqueOrLive(names []string, branch bool, consume func(*os.File)) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
		if branch {
			consume(file)
		}
	}
	return nil
}

// The inner cycle can bring more statuses back to its header. Closing after
// it must settle every live incoming state before the outer backedge.
func cleanupAfterInnerLoop(names []string, repeats int, consume func(*os.File)) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close()
		for range repeats {
			consume(file)
		}
		_ = file.Close()
	}
	return nil
}

func innerLoopMayNotRun(names []string, repeats int) error {
	for _, name := range names {
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer file.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
		for range repeats {
			_ = file.Close()
		}
	}
	return nil
}
