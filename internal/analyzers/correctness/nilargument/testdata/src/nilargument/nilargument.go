package nilargument

// Each helper below is summarized by the lifecycle pass; the check reads
// what the helper requires of its arguments and judges the caller's state
// at the call. The accepted cases pin the boundary: nil on one branch, nil
// replaced before the call, a callee that checks first or dereferences on
// some path only, and an interface filled from a nil pointer.

type node struct {
	value int
	next  *node
}

type response struct {
	body   *node
	reader interface{ Read() int }
}

// Length dereferences its argument on every path.
func Length(n *node) int { return n.value }

// Guarded checks before it dereferences.
func Guarded(n *node) int {
	if n == nil {
		return 0
	}
	return n.value
}

// Sometimes dereferences on one path only.
func Sometimes(n *node, enabled bool) int {
	if enabled {
		return n.value
	}
	return 0
}

// Keep only stores the argument.
var kept *node

func Keep(n *node) { kept = n }

// BodyValue dereferences the body inside the response.
func BodyValue(r *response) int { return r.body.value }

// Initializing the field within the callee means its later dereference does
// not require the caller to supply a non-nil field in the incoming object.
func InitializeThenBodyValue(r *response) int {
	r.body = &node{value: 1}
	return r.body.value
}

// Read invokes the reader inside the response.
func Read(r *response) int { return r.reader.Read() }

// Through hands the argument to Length.
func Through(n *node) int { return Length(n) }

//gohawk:example flagged
func literalNil() int {
	return Length(nil) // want "argument 1 is nil, and Length dereferences it on every path"
}

//gohawk:example end

func variableNil() int {
	var n *node
	return Length(n) // want "argument 1 is nil, and Length dereferences it on every path"
}

func nilFieldOfLocal() int {
	r := &response{}
	return BodyValue(r) // want "field body of argument 1 is nil, and BodyValue dereferences it on every path"
}

func calleeInitializesNilField() int {
	r := &response{}
	return InitializeThenBodyValue(r)
}

func nilFieldSet(r *response) int {
	r.body = nil
	return BodyValue(r) // want "field body of argument 1 is nil, and BodyValue dereferences it on every path"
}

func throughHelper() int {
	return Through(nil) // want "argument 1 is nil, and Through dereferences it on every path"
}

//gohawk:example ok
func nilOnOneBranch(ready bool) int {
	var n *node
	if ready {
		n = &node{value: 1}
	}
	return Length(n)
}

//gohawk:example end

func replacedBeforeCall() int {
	var n *node
	n = &node{value: 1}
	return Length(n)
}

func guardedCallee() int {
	return Guarded(nil)
}

func sometimesCallee(enabled bool) int {
	return Sometimes(nil, enabled)
}

func onlyStored() {
	Keep(nil)
}

func fieldSetBeforeCall() int {
	r := &response{}
	r.body = &node{value: 1}
	return BodyValue(r)
}

func fieldSetOnOneBranch(ready bool) int {
	r := &response{}
	if ready {
		r.body = &node{value: 1}
	}
	return BodyValue(r)
}

// An interface slot filled from a nil pointer is not a nil interface, and
// the graph does not tell the two apart, so an interface slot is never a
// witness.
type reader struct{}

func (*reader) Read() int { return 0 }

func interfaceFromNilPointer() int {
	var r *reader
	return Read(&response{reader: r})
}

func nilInterface() int {
	return Read(&response{})
}

// A parameter's field is not known to be nil: the caller never wrote it.
func parameterField(r *response) int {
	return BodyValue(r)
}
