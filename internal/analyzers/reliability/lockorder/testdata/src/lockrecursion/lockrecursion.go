// Package lockrecursion is a cost regression fixture, not a diagnostic one.
//
// Mutually recursive helpers make the number of routes through a call graph
// explode. The cycle guard keeps the walk finite, but a memo cannot retain an
// answer the guard cut short, so an unbounded "does this callee release my
// lock?" search re-walks the graph once per route instead of once per
// function. This package took over forty-five seconds to analyze before the
// search was bounded and about a tenth of a second after.
//
// Nothing here is expected to be reported: run acquires one lock and releases
// it on every path, and the recursive methods lock nothing at all.
package lockrecursion

import "sync"

type recursiveTree struct {
	mu sync.Mutex
}

func (t *recursiveTree) visit0(depth int) {
	if depth == 0 {
		return
	}
	t.visit1(depth - 1)
	t.visit2(depth - 1)
	t.visit3(depth - 1)
	t.visit4(depth - 1)
}

func (t *recursiveTree) visit1(depth int) {
	if depth == 0 {
		return
	}
	t.visit8(depth - 1)
	t.visit9(depth - 1)
	t.visit10(depth - 1)
	t.visit11(depth - 1)
}

func (t *recursiveTree) visit2(depth int) {
	if depth == 0 {
		return
	}
	t.visit15(depth - 1)
	t.visit16(depth - 1)
	t.visit17(depth - 1)
	t.visit18(depth - 1)
}

func (t *recursiveTree) visit3(depth int) {
	if depth == 0 {
		return
	}
	t.visit0(depth - 1)
	t.visit1(depth - 1)
	t.visit2(depth - 1)
	t.visit3(depth - 1)
}

func (t *recursiveTree) visit4(depth int) {
	if depth == 0 {
		return
	}
	t.visit7(depth - 1)
	t.visit8(depth - 1)
	t.visit9(depth - 1)
	t.visit10(depth - 1)
}

func (t *recursiveTree) visit5(depth int) {
	if depth == 0 {
		return
	}
	t.visit14(depth - 1)
	t.visit15(depth - 1)
	t.visit16(depth - 1)
	t.visit17(depth - 1)
}

func (t *recursiveTree) visit6(depth int) {
	if depth == 0 {
		return
	}
	t.visit21(depth - 1)
	t.visit0(depth - 1)
	t.visit1(depth - 1)
	t.visit2(depth - 1)
}

func (t *recursiveTree) visit7(depth int) {
	if depth == 0 {
		return
	}
	t.visit6(depth - 1)
	t.visit7(depth - 1)
	t.visit8(depth - 1)
	t.visit9(depth - 1)
}

func (t *recursiveTree) visit8(depth int) {
	if depth == 0 {
		return
	}
	t.visit13(depth - 1)
	t.visit14(depth - 1)
	t.visit15(depth - 1)
	t.visit16(depth - 1)
}

func (t *recursiveTree) visit9(depth int) {
	if depth == 0 {
		return
	}
	t.visit20(depth - 1)
	t.visit21(depth - 1)
	t.visit0(depth - 1)
	t.visit1(depth - 1)
}

func (t *recursiveTree) visit10(depth int) {
	if depth == 0 {
		return
	}
	t.visit5(depth - 1)
	t.visit6(depth - 1)
	t.visit7(depth - 1)
	t.visit8(depth - 1)
}

func (t *recursiveTree) visit11(depth int) {
	if depth == 0 {
		return
	}
	t.visit12(depth - 1)
	t.visit13(depth - 1)
	t.visit14(depth - 1)
	t.visit15(depth - 1)
}

func (t *recursiveTree) visit12(depth int) {
	if depth == 0 {
		return
	}
	t.visit19(depth - 1)
	t.visit20(depth - 1)
	t.visit21(depth - 1)
	t.visit0(depth - 1)
}

func (t *recursiveTree) visit13(depth int) {
	if depth == 0 {
		return
	}
	t.visit4(depth - 1)
	t.visit5(depth - 1)
	t.visit6(depth - 1)
	t.visit7(depth - 1)
}

func (t *recursiveTree) visit14(depth int) {
	if depth == 0 {
		return
	}
	t.visit11(depth - 1)
	t.visit12(depth - 1)
	t.visit13(depth - 1)
	t.visit14(depth - 1)
}

func (t *recursiveTree) visit15(depth int) {
	if depth == 0 {
		return
	}
	t.visit18(depth - 1)
	t.visit19(depth - 1)
	t.visit20(depth - 1)
	t.visit21(depth - 1)
}

func (t *recursiveTree) visit16(depth int) {
	if depth == 0 {
		return
	}
	t.visit3(depth - 1)
	t.visit4(depth - 1)
	t.visit5(depth - 1)
	t.visit6(depth - 1)
}

func (t *recursiveTree) visit17(depth int) {
	if depth == 0 {
		return
	}
	t.visit10(depth - 1)
	t.visit11(depth - 1)
	t.visit12(depth - 1)
	t.visit13(depth - 1)
}

func (t *recursiveTree) visit18(depth int) {
	if depth == 0 {
		return
	}
	t.visit17(depth - 1)
	t.visit18(depth - 1)
	t.visit19(depth - 1)
	t.visit20(depth - 1)
}

func (t *recursiveTree) visit19(depth int) {
	if depth == 0 {
		return
	}
	t.visit2(depth - 1)
	t.visit3(depth - 1)
	t.visit4(depth - 1)
	t.visit5(depth - 1)
}

func (t *recursiveTree) visit20(depth int) {
	if depth == 0 {
		return
	}
	t.visit9(depth - 1)
	t.visit10(depth - 1)
	t.visit11(depth - 1)
	t.visit12(depth - 1)
}

func (t *recursiveTree) visit21(depth int) {
	if depth == 0 {
		return
	}
	t.visit16(depth - 1)
	t.visit17(depth - 1)
	t.visit18(depth - 1)
	t.visit19(depth - 1)
}

// run is the shape that makes the search expensive: a lock held across one
// call into the recursive family.
func run(t *recursiveTree) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.visit0(6)
}
