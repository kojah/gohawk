package resourcelifetime

import "os"

// A function literal that captures a resource is judged by where it runs. One
// that is called or deferred runs in this frame, so its body is as readable as
// a named callee's and the question is the same: does it keep the resource? A
// literal that only reads through it leaves the obligation here, and a literal
// that parks it somewhere has taken it over. A started literal is different:
// its release cannot be ordered against this function's returns, so the
// resource is beyond what this flow can judge.
//
// A literal is never summarized, having no object of its own, so the question
// a summary answers for a named callee is asked of the body directly.

var literalSink struct{ file *os.File }

// fileReadThroughCalledLiteral leaks: the literal only reads the file, so this
// function still owns it and never closes it.
func fileReadThroughCalledLiteral(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	func() { _ = file }()
	return nil
}

// fileHeldByDeferredLiteral leaks for the same reason: the deferred literal
// mentions the file without releasing it, which is the shape a cleanup takes
// when it removes a path and forgets the handle.
func fileHeldByDeferredLiteral(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	defer func() { _ = file }()
	return nil
}

// fileParkedByCalledLiteral is accepted: the literal keeps the file somewhere
// this function cannot reach, so the obligation went with it.
func fileParkedByCalledLiteral(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	func() { literalSink.file = file }()
	return nil
}

// fileClosedByDeferredLiteral is accepted: the deferred literal closes it, and
// that release is proved before the capture is ever judged.
func fileClosedByDeferredLiteral(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return nil
}

// fileClosedByStartedLiteral is accepted, and would be accepted even if the
// literal closed nothing: a release on another goroutine cannot be ordered
// against this function's returns, so the flow declines rather than guesses.
func fileClosedByStartedLiteral(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	go func() { _ = file.Close() }()
	return nil
}
