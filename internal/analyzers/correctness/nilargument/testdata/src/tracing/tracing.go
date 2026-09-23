package tracing

// The trace of a nil proof lists the earlier calls whose summaries the
// graph did not apply. The interface call cannot reach the local, so the
// proof still holds and the call it lists is the one a reader checks.

type node struct{ value int }

type runner interface{ Run() }

func Length(n *node) int { return n.value }

func afterInterfaceCall(r runner) int {
	var n *node
	r.Run()
	return Length(n) // want "argument 1 is nil, and Length dereferences it on every path"
}
