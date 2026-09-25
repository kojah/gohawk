// A main package cannot be imported, so even an exported field is closed.
package main

import (
	"context"
	"errors"
	"sync"
)

type negentropy struct{ Deltas chan int }

func newNegentropy() *negentropy { return &negentropy{Deltas: make(chan int, 100)} }

func (n *negentropy) Run(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.New("cancelled")
	}
	n.Deltas <- 1
	close(n.Deltas)
	return nil
}

func main() {
	ctx := context.Background()
	tpn := newNegentropy()
	var err error
	var wg sync.WaitGroup
	wg.Go(func() { err = tpn.Run(ctx) })
	wg.Go(func() {
		for delta := range tpn.Deltas { // want "range can wait forever: Run returns an error without closing the channel"
			_ = delta
		}
	})
	wg.Wait()
	_ = err
}
