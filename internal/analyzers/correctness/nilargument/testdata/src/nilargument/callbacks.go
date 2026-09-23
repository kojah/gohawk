package nilargument

// A variable a callback assigns is not nil after the call that ran the
// callback, however the call reached it: a helper that invokes its
// callback parameter, one that invokes it through an interface, a callback
// handed to code the graph cannot follow, or a variable whose address
// escaped before an unresolved call. A callback that never writes the
// variable leaves it nil.

func visit(f func()) { f() }

type visitor interface{ Visit() }

type visitFunc func()

func (f visitFunc) Visit() { f() }

func walk(v visitor) { v.Visit() }

func inspect(f func()) { walk(visitFunc(f)) }

var runner func(func())

var saved **node

func assignedByInvokedCallback() int {
	var found *node
	visit(func() { found = &node{} })
	return Length(found)
}

func assignedThroughInterfaceCallback() int {
	var found *node
	inspect(func() { found = &node{} })
	return Length(found)
}

func assignedByUnresolvedCallback() int {
	var found *node
	runner(func() { found = &node{} })
	return Length(found)
}

func assignedAfterAddressEscaped() int {
	var found *node
	saved = &found
	runner(nil)
	return Length(found)
}

func callbackLeavesNil() int {
	var found *node
	visit(func() {})
	return Length(found) // want "argument 1 is nil, and Length dereferences it on every path"
}
