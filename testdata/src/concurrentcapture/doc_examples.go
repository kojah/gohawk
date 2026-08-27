package concurrentcapture

import "sync"

//gohawk:example flagged
func collect(items []int) error {
	var err error
	for range items {
		go func() {
			err = fetch() // want "captured local err is mutated by goroutines launched repeatedly"
		}()
	}
	return err
}

//gohawk:example end

//gohawk:example ok
func collectSafely(items []int) error {
	var group sync.WaitGroup
	errs := make(chan error, len(items))
	for range items {
		group.Add(1)
		go func() {
			defer group.Done()
			errs <- fetch()
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

//gohawk:example end

func fetch() error { return nil }
