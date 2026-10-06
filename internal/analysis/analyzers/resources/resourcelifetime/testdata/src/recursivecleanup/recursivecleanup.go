// Package recursivecleanup is a cost regression fixture, not a diagnostic one.
//
// Mutually recursive helpers make the number of routes through a call graph
// explode. The cycle guard keeps the release search finite, but a memo cannot
// retain an answer the guard cut short, so an unbounded search re-walks the
// graph once per route rather than once per function. This package took longer
// than a minute to analyze before the search was bounded and about a third of
// a second after.
//
// Nothing here is expected to be reported: the file is closed on every path,
// and the recursive helpers only pass it along.
package recursivecleanup

import "os"

type holder struct {
	file *os.File
}

func visit0(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit1(h, depth-1)
	visit2(h, depth-1)
	visit3(h, depth-1)
	visit4(h, depth-1)
	visit5(h, depth-1)
	visit6(h, depth-1)
	visit7(h, depth-1)
	visit8(h, depth-1)
}

func visit1(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit8(h, depth-1)
	visit9(h, depth-1)
	visit10(h, depth-1)
	visit11(h, depth-1)
	visit12(h, depth-1)
	visit13(h, depth-1)
	visit0(h, depth-1)
	visit1(h, depth-1)
}

func visit2(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit1(h, depth-1)
	visit2(h, depth-1)
	visit3(h, depth-1)
	visit4(h, depth-1)
	visit5(h, depth-1)
	visit6(h, depth-1)
	visit7(h, depth-1)
	visit8(h, depth-1)
}

func visit3(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit8(h, depth-1)
	visit9(h, depth-1)
	visit10(h, depth-1)
	visit11(h, depth-1)
	visit12(h, depth-1)
	visit13(h, depth-1)
	visit0(h, depth-1)
	visit1(h, depth-1)
}

func visit4(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit1(h, depth-1)
	visit2(h, depth-1)
	visit3(h, depth-1)
	visit4(h, depth-1)
	visit5(h, depth-1)
	visit6(h, depth-1)
	visit7(h, depth-1)
	visit8(h, depth-1)
}

func visit5(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit8(h, depth-1)
	visit9(h, depth-1)
	visit10(h, depth-1)
	visit11(h, depth-1)
	visit12(h, depth-1)
	visit13(h, depth-1)
	visit0(h, depth-1)
	visit1(h, depth-1)
}

func visit6(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit1(h, depth-1)
	visit2(h, depth-1)
	visit3(h, depth-1)
	visit4(h, depth-1)
	visit5(h, depth-1)
	visit6(h, depth-1)
	visit7(h, depth-1)
	visit8(h, depth-1)
}

func visit7(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit8(h, depth-1)
	visit9(h, depth-1)
	visit10(h, depth-1)
	visit11(h, depth-1)
	visit12(h, depth-1)
	visit13(h, depth-1)
	visit0(h, depth-1)
	visit1(h, depth-1)
}

func visit8(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit1(h, depth-1)
	visit2(h, depth-1)
	visit3(h, depth-1)
	visit4(h, depth-1)
	visit5(h, depth-1)
	visit6(h, depth-1)
	visit7(h, depth-1)
	visit8(h, depth-1)
}

func visit9(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit8(h, depth-1)
	visit9(h, depth-1)
	visit10(h, depth-1)
	visit11(h, depth-1)
	visit12(h, depth-1)
	visit13(h, depth-1)
	visit0(h, depth-1)
	visit1(h, depth-1)
}

func visit10(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit1(h, depth-1)
	visit2(h, depth-1)
	visit3(h, depth-1)
	visit4(h, depth-1)
	visit5(h, depth-1)
	visit6(h, depth-1)
	visit7(h, depth-1)
	visit8(h, depth-1)
}

func visit11(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit8(h, depth-1)
	visit9(h, depth-1)
	visit10(h, depth-1)
	visit11(h, depth-1)
	visit12(h, depth-1)
	visit13(h, depth-1)
	visit0(h, depth-1)
	visit1(h, depth-1)
}

func visit12(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit1(h, depth-1)
	visit2(h, depth-1)
	visit3(h, depth-1)
	visit4(h, depth-1)
	visit5(h, depth-1)
	visit6(h, depth-1)
	visit7(h, depth-1)
	visit8(h, depth-1)
}

func visit13(h *holder, depth int) {
	if depth == 0 {
		return
	}
	visit8(h, depth-1)
	visit9(h, depth-1)
	visit10(h, depth-1)
	visit11(h, depth-1)
	visit12(h, depth-1)
	visit13(h, depth-1)
	visit0(h, depth-1)
	visit1(h, depth-1)
}

// run is the shape that makes the search expensive: a resource whose release
// must be proven, held across one call into the recursive family.
func run() error {
	file, err := os.Open("input")
	if err != nil {
		return err
	}
	defer file.Close()
	visit0(&holder{file: file}, 6)
	return nil
}
