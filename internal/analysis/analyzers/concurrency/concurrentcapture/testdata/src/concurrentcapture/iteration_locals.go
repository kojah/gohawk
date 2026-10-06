//go:build go1.22

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

func rangeHeaderLocal(items []int) {
	for index, value := range items {
		go func() { index++; value = 7; _ = value }()
	}
}

func rangeHeaderSharedAssignment(items []int) {
	var value int
	for _, value = range items {
		go func() { value = 7; _ = value }() // want "captured local value is mutated by goroutines launched repeatedly"
	}
}

func nestedLoopSharesRangeHeader(items []int) {
	for _, value := range items {
		for range items {
			go func() { value = 7; _ = value }() // want "captured local value is mutated by goroutines launched repeatedly"
		}
	}
}

func rangeHeaderSharedMapAlias(items []map[string]int) {
	for _, value := range items {
		go func() { value["key"] = 7 }() // want "captured local value is mutated by goroutines launched repeatedly"
	}
}

func rangeHeaderPointerReassignment(items []*int) {
	for _, value := range items {
		go func() { value = nil; _ = value }()
	}
}

func forHeaderPostSharesWithWorker() {
	for index := 0; index < 5; index++ {
		go func() { index++ }() // want "captured local index is mutated by goroutines launched repeatedly"
	}
}
