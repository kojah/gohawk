package goroutineownership

import (
	"context"
	"sync"
)

// A worker bounded by a context field of the caller-owned receiver it
// captures. The receiver's owner installed that context, so it owns the
// cancellation; this is uncertainty, not a join. A context the spawning
// function installs itself, a publishing worker, or a receiver allocated
// locally does not qualify.

type segmentDownloader struct {
	ctx     context.Context
	results chan int
	queue   []int
}

func (d *segmentDownloader) next() (int, bool, error) {
	if len(d.queue) == 0 {
		return 0, true, context.Canceled
	}
	index := d.queue[0]
	d.queue = d.queue[1:]
	return index, false, nil
}

func (d *segmentDownloader) segment(index int) error {
	select {
	case <-d.ctx.Done():
		return d.ctx.Err()
	default:
	}
	return nil
}

func (d *segmentDownloader) startBoundedByReceiverContext() error {
	var wg sync.WaitGroup
	for {
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()
		default:
		}
		index, end, err := d.next()
		if err != nil {
			if end {
				break
			}
			continue
		}
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_ = d.segment(index)
		}(index)
	}
	wg.Wait()
	return nil
}

func (d *segmentDownloader) startWithContextInstalledHere() error {
	d.ctx = context.Background()
	var wg sync.WaitGroup
	for {
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()
		default:
		}
		index, end, err := d.next()
		if err != nil {
			if end {
				break
			}
			continue
		}
		wg.Add(1)
		go func(index int) { // want "goroutine is not joined on every return path"
			defer wg.Done()
			_ = d.segment(index)
		}(index)
	}
	wg.Wait()
	return nil
}

func (d *segmentDownloader) startPublishingWorker() error {
	var wg sync.WaitGroup
	for {
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()
		default:
		}
		index, end, err := d.next()
		if err != nil {
			if end {
				break
			}
			continue
		}
		wg.Add(1)
		go func(index int) { // want "goroutine is not joined on every return path"
			defer wg.Done()
			_ = d.segment(index)
			d.results <- index
		}(index)
	}
	wg.Wait()
	return nil
}

func startOnLocalReceiver(ctx context.Context) error {
	d := &segmentDownloader{ctx: ctx}
	var wg sync.WaitGroup
	for {
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()
		default:
		}
		index, end, err := d.next()
		if err != nil {
			if end {
				break
			}
			continue
		}
		wg.Add(1)
		go func(index int) { // want "goroutine is not joined on every return path"
			defer wg.Done()
			_ = d.segment(index)
		}(index)
	}
	wg.Wait()
	return nil
}
