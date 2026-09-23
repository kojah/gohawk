package lifecyclefacts

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// A fresh resource handed straight back is an owned result. Anything that
// keeps, touches, or may already have released it before the return is not,
// and neither is a caller's own resource handed back or a type the caller
// cannot release. A resource forwarded from an unsummarized private helper
// has no trusted acquisition and makes no claim.
func TestLifecycleSummaryOwnedResults(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

import (
	"errors"
	"io"
	"os"
)

var registry []*os.File

func OpenFresh(path string) (*os.File, error) { return os.Open(path) }
func OpenReader(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return f, nil
}
func OpenClosedOnFailure(path string, ok bool) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if !ok {
		f.Close()
		return nil, errors.New("rejected")
	}
	return f, nil
}
func OpenRegistered(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	registry = append(registry, f)
	return f, nil
}
func OpenSeeked(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	_, _ = f.Seek(0, 0)
	return f, nil
}
func OpenWithCleanup(path string) (*os.File, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, func() {}, err
	}
	return f, func() { _ = f.Close() }, nil
}
func OpenMaybeClosed(path string, flush bool) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if flush {
		_ = f.Close()
	}
	return f, nil
}
func OpenView(file *os.File) *os.File { return file }
func OpenPlain(path string) (io.Reader, error) { return os.Open(path) }
func openIndirectly(path string) (*os.File, error) { return os.Open(path) }
func OpenForwarded(path string) (*os.File, error) { return openIndirectly(path) }
func OpenFromFD(fd uintptr) *os.File { return os.NewFile(fd, "fd") }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string]ParameterMask{
		"OpenFresh":           parameterMaskFor(0),
		"OpenReader":          parameterMaskFor(0),
		"OpenClosedOnFailure": parameterMaskFor(0),
		"OpenRegistered":      0,
		"OpenSeeked":          0,
		"OpenWithCleanup":     0,
		"OpenMaybeClosed":     0,
		"OpenView":            0,
		"OpenPlain":           parameterMaskFor(0),
		"OpenForwarded":       0,
		"OpenFromFD":          parameterMaskFor(0),
	} {
		fact := summarize(pass, newRetentionCache(), pkg.Func(name))
		if fact.OwnedResults != want {
			t.Errorf("%s: OwnedResults = %v, want %v", name, fact.OwnedResults, want)
		}
	}
}
