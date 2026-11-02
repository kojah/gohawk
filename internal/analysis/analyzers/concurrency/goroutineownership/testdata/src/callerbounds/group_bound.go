package callerbounds

import (
	"context"
	"sync"
)

// A watcher whose only blocking operation is Wait on a caller-owned
// WaitGroup ends when the group settles. Its lifetime is bounded by the
// group's external owner, so leaving it unjoined is unknown, not a leak.
// https://github.com/kojah/gohawk/issues/57

type boundGroup struct{ inFlight sync.WaitGroup }

func externalGroupWatcher(c *boundGroup, ctx context.Context) error {
	done := make(chan struct{})
	go func() { c.inFlight.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func paramGroupWatcher(group *sync.WaitGroup) {
	done := make(chan struct{})
	go func(g *sync.WaitGroup) { g.Wait(); close(done) }(group)
}

var sharedGroup sync.WaitGroup

func globalGroupWatcher() {
	done := make(chan struct{})
	go func() { sharedGroup.Wait(); close(done) }()
}

// A group built in this frame keeps the bound: its settlement is this
// function's own responsibility, so the unjoined worker still leaks.
func localGroupWatcher() {
	group := new(sync.WaitGroup)
	done := make(chan struct{})
	go func() { group.Wait(); close(done) }() // want "goroutine is not joined on every return path"
}

// The external-group bound covers only the Wait itself. A worker that can
// still block after the group settles — here on a local channel receive —
// keeps the unjoined diagnostic.
// https://github.com/kojah/gohawk/pull/58#discussion_r4437363032
func externalGroupThenBlocking() {
	ch := make(chan int)
	go func() { sharedGroup.Wait(); <-ch; close(ch) }() // want "goroutine is not joined on every return path"
}
