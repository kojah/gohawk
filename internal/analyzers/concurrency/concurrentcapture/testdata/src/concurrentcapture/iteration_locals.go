package concurrentcapture

func iterationLocal(items []int) {
	for range items {
		err := work()
		if err != nil {
			return
		}
		go func() { err = work() }()
	}
}

func shortDeclarationReusesOuter(items []int) {
	var err error
	for range items {
		err = work()
		go func() {
			err = work() // want "captured local err is mutated by goroutines launched repeatedly"
			_ = err
		}()
	}
}

func nestedLoopSharesOuterIteration(items []int) {
	for range items {
		var err error
		for range items {
			go func() {
				err = work() // want "captured local err is mutated by goroutines launched repeatedly"
				_ = err
			}()
		}
	}
}
