package lifecyclefacts

import (
	"go/types"
	"slices"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// A deferred completion is exported at the path the proof names: a literal
// or a deferred helper closing a field claims that field, one closing the
// parameter itself sets the mask, and one whose path the proof cannot name,
// because different returns settle different fields or the receiver comes
// from a call, claims nothing rather than the whole parameter.
func TestLifecycleSummaryDeferredCompletionPaths(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type closer struct{}

func (*closer) Close() {}

type owner struct{ body, other *closer }

func (o *owner) pick() *closer { return o.body }

func closeBody(o *owner) { o.body.Close() }

func Field(value *owner)  { defer func() { value.body.Close() }() }
func Helper(value *owner) { defer closeBody(value) }
func Self(value *closer)  { defer func() { value.Close() }() }
func Either(value *owner, enabled bool) {
	defer func() {
		if enabled {
			value.body.Close()
		} else {
			value.other.Close()
		}
	}()
}
func Picked(value *owner) { defer func() { value.pick().Close() }() }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string]struct {
		closed bool
		paths  []string
	}{
		"Field":  {paths: []string{"field:0"}},
		"Helper": {paths: []string{"field:0"}},
		"Self":   {closed: true, paths: []string{""}},
		"Either": {},
		"Picked": {},
	} {
		fact := summarize(pass, pkg.Func(name))
		var paths []string
		for _, discharge := range fact.Discharges {
			if discharge.Parameter == 0 && discharge.Method == "Close" {
				paths = append(paths, discharge.Path)
			}
		}
		if fact.Closed.contains(0) != want.closed || !slices.Equal(paths, want.paths) {
			t.Errorf("%s Closed = %t, discharge paths = %q; want %t, %q", name, fact.Closed.contains(0), paths, want.closed, want.paths)
		}
	}
}
