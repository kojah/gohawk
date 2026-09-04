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

// closeThroughHelper releases the file it is given. It is a local function so
// the walk reads its body here rather than depending on whether a dependency's
// bodies happen to be available.
func closeThroughHelper(file *os.File) error { return file.Close() }

// parkThroughHelper keeps the file where the caller cannot reach it.
func parkThroughHelper(file *os.File) { literalSink.file = file }

// fileClosedByDeferredLiteralHelper is accepted: the deferred literal releases
// the file through a helper rather than closing it inline.
func fileClosedByDeferredLiteralHelper(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = closeThroughHelper(file) }()
	return nil
}

// fileParkedByDeferredLiteralHelper is accepted for the other reason: the
// literal hands the file to a helper that keeps it, so the obligation moved.
func fileParkedByDeferredLiteralHelper(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { parkThroughHelper(file) }()
	return nil
}

// readThroughHelper neither releases the file nor keeps it.
func readThroughHelper(file *os.File) bool { return file != nil }

// fileReadByDeferredLiteralHelper leaks: neither the literal nor the helper it
// calls releases or keeps the file, so this function still owns it.
func fileReadByDeferredLiteralHelper(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	defer func() { _ = readThroughHelper(file) }()
	return nil
}
