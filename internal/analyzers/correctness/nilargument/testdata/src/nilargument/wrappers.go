package nilargument

// A function that wraps its argument and hands the wrapper to a helper
// requires what the helper requires of the wrapped field, because at the
// call that field certainly holds the argument. A wrapper filled on one
// branch only proves nothing about the argument.

type wrapper struct{ inner *node }

func wrappedLength(w *wrapper) int { return w.inner.value }

// LengthOfWrapped dereferences its argument through a wrapper on every path.
func LengthOfWrapped(n *node) int { return wrappedLength(&wrapper{inner: n}) }

// LengthOfMaybeWrapped wraps its argument on one branch only.
func LengthOfMaybeWrapped(n *node, wrap bool) int {
	w := &wrapper{inner: &node{}}
	if wrap {
		w.inner = n
	}
	return wrappedLength(w)
}

func nilThroughWrapper() int {
	return LengthOfWrapped(nil) // want "argument 1 is nil, and LengthOfWrapped dereferences it on every path"
}

func nilThroughMaybeWrapper() int {
	return LengthOfMaybeWrapped(nil, true)
}
