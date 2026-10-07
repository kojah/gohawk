package lifecyclefacts

import (
	"go/types"
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestNonReturningMethodsDoNotInventCleanupContracts(t *testing.T) {
	contracts := contractsFor(t, `
package lifecyclefactstest
import "os"
type Pending struct { file *os.File }
func NewPending(path string) (*Pending, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	return &Pending{file:f}, nil
}
func (p *Pending) Configure() { panic("not implemented") }
type Real struct { file *os.File }
func NewReal(path string) (*Real, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	return &Real{file:f}, nil
}
func (r *Real) Stop() { r.file.Close() }
func (r *Real) Configure() { panic("not implemented") }
`)
	if contract := contracts["Pending"]; contract != nil {
		t.Errorf("panic-only method invented cleanup contract: %v", contract)
	}
	contract := contracts["Real"]
	if contract == nil || len(contract.Methods) != 1 || contract.Methods[0] != "Stop" {
		t.Errorf("real cleanup contract = %v, want only Stop", contract)
	}
}

func TestLifecycleMasksRequireCompletionWitness(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest
type closer interface { Close() error }
type holder struct { value closer }
func panicOnly(c closer) { panic("not implemented") }
func loopOnly(c closer) { for {} }
func panicUnlessNil(c closer) { if c == nil { return }; panic("not implemented") }
func panicOwner(c closer) *holder { panic("not implemented") }
func (h *holder) PanicStore(c closer) { panic("not implemented") }
func closeNormally(c closer) { c.Close() }
`)
	pass := &analysis.Pass{Pkg: pkg.Pkg, ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for _, name := range []string{"panicOnly", "loopOnly", "panicUnlessNil", "panicOwner"} {
		fact := summarize(pass, pkg.Func(name))
		for _, mask := range lifecycleMasks {
			if got := mask.mask(&fact); got != 0 {
				t.Errorf("%s invented %s mask %#x", name, mask.name, got)
			}
		}
		if fact.SynchronouslyInvoked() != 0 {
			t.Errorf("%s invented synchronous invocation", name)
		}
	}
	method := pkg.Prog.LookupMethod(types.NewPointer(pkg.Type("holder").Type()), pkg.Pkg, "PanicStore")
	if fact := summarize(pass, method); fact.ReceiverStore() != 0 {
		t.Errorf("panic-only receiver method invented receiver-store mask %#x", fact.ReceiverStore())
	}
	fact := summarize(pass, pkg.Func("closeNormally"))
	if !fact.MethodMask("Close").contains(0) {
		t.Error("real Close witness did not produce Closed fact")
	}
}

// The kept-contents claim is loose about whether and exact about where: a
// field stored, sent, captured, started, or returned is claimed at its own
// path, through a visible helper as well as directly; a basic-typed load, a
// read, or a call into a visible body that keeps nothing claims no path; and
// a non-aggregate parameter carries no claim at all. A field selected from a
// local copy of the pointee is still that field, and a parameter Retained
// outright, including one whose whole value is copied out, needs no claim
// of its own.
func TestLifecycleSummaryKeptContents(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

type closer struct{ id int }

func (c *closer) Close() error { return nil }

type pair struct {
	first, second *closer
	name          string
}
type nested struct{ inner pair }
type sink interface{ Add(any) }

var (
	saved    *closer
	savedAll pair
	registry sink
	names    []string
	handles  chan *closer
)

func KeepFirst(p *pair)              { saved = p.first }
func KeepCopy(p *pair)               { c := *p; saved = c.second }
func KeepWhole(p *pair)              { savedAll = *p }
func Publish(p *pair)                { registry.Add(p.second) }
func Send(p *pair)                   { handles <- p.first }
func Capture(p *pair)                { defer func() { _ = p.second }() }
func Start(p *pair)                  { go use(p.first) }
func use(c *closer)                  { _ = c }
func ViaHelper(p *pair)              { KeepFirst(p) }
func ViaFieldHelper(p *pair)         { keep(p.second) }
func keep(c *closer)                 { saved = c }
func ByValue(p pair)                 { saved = p.first }
func First(p *pair) *closer          { return p.first }
func Nested(n *nested)               { saved = n.inner.second }
func KeepName(p *pair)               { names = append(names, p.name) }
func Inspect(p *pair) bool           { return p.first != nil }
func CloseSecond(p *pair) error      { return p.second.Close() }
func ViaReader(p *pair)              { read(p) }
func read(p *pair)                   { _ = p.first }
func NotAggregate(c *closer)         { saved = c }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string][]string{
		"KeepFirst":      {"field:0"},
		"KeepCopy":       {"field:1"},
		"KeepWhole":      nil,
		"Publish":        {"field:1"},
		"Send":           {"field:0"},
		"Capture":        nil,
		"Start":          {"field:0"},
		"ViaHelper":      {"field:0"},
		"ViaFieldHelper": {"field:1"},
		"ByValue":        {"field:0"},
		"First":          {"field:0"},
		"Nested":         {"field:0/field:1"},
		"KeepName":       nil,
		"Inspect":        nil,
		"CloseSecond":    nil,
		"ViaReader":      nil,
		"NotAggregate":   nil,
	} {
		fact := summarize(pass, pkg.Func(name))
		var got []string
		for _, kept := range fact.Kept() {
			if kept.Parameter == 0 {
				got = append(got, kept.Path)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s Kept paths = %q, want %q (retained %t)", name, got, want, fact.Retained().contains(0))
		}
	}
}

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
		if fact.MethodMask("Close").contains(0) != want.closed || !slices.Equal(paths, want.paths) {
			t.Errorf("%s Closed = %t, discharge paths = %q; want %t, %q", name, fact.MethodMask("Close").contains(0), paths, want.closed, want.paths)
		}
	}
}

// A struct parameter is passed by value and spilled into a local cell before
// its fields are selected. The summary must still see a cleanup call on the
// field, and claim it at the field's own path rather than for the whole
// parameter, for the parameter itself, a value receiver, a local copy, an
// array, and an array inside a struct alike. A branch without the call, or a
// call on a field of some other local, proves nothing about the parameter.
func TestLifecycleSummaryClosesFieldOfByValueParameter(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

import "os"

type job struct{ out *os.File }

func Direct(j job) error { return j.out.Close() }
func Copied(j job) error { k := j; return k.out.Close() }
func (j job) Finish() error { return j.out.Close() }
func OneBranch(j job, flag bool) error {
	if flag {
		return nil
	}
	return j.out.Close()
}
func Sibling(j job, other *os.File) error {
	k := job{out: other}
	return k.out.Close()
}
func Indexed(files [2]*os.File) error { return files[0].Close() }
type batch struct{ files [2]*os.File }
func Nested(b batch) error { return b.files[0].Close() }
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	cases := map[string]struct {
		function *ssa.Function
		path     string
		want     bool
	}{
		"Direct":    {pkg.Func("Direct"), "field:0", true},
		"Copied":    {pkg.Func("Copied"), "field:0", true},
		"Finish":    {pkg.Prog.LookupMethod(pkg.Type("job").Type(), pkg.Pkg, "Finish"), "field:0", true},
		"OneBranch": {pkg.Func("OneBranch"), "field:0", false},
		"Sibling":   {pkg.Func("Sibling"), "field:0", false},
		"Indexed":   {pkg.Func("Indexed"), "index:0", true},
		"Nested":    {pkg.Func("Nested"), "field:0/index:0", true},
	}
	for name, test := range cases {
		fact := summarize(pass, test.function)
		got := slices.Contains(fact.Discharges, Discharge{Parameter: 0, Method: "Close", Path: test.path})
		if got != test.want {
			t.Errorf("%s: discharge of Close at %q = %t, want %t (fact %+v)", name, test.path, got, test.want, fact.Discharges)
		}
		if fact.MethodMask("Close") != 0 {
			t.Errorf("%s: a field cleanup must not claim the whole parameter", name)
		}
	}
}

// A loop that releases values drawn from a parameter is a may-claim about that
// parameter alone. A flag-conditional release is not a loop, and a loop that
// releases some other parameter's elements says nothing about this one.
func TestLifecycleSummaryRecordsReleaseInLoop(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `
package lifecyclefactstest

import "os"

func CloseEach(name string, files ...*os.File) {
	for _, file := range files {
		if file != nil {
			_ = file.Close()
		}
	}
}
func CloseSlice(files []*os.File) {
	for _, file := range files {
		_ = file.Close()
	}
}
func CloseWhenFlagged(file *os.File, flagged bool) {
	if flagged {
		_ = file.Close()
	}
}
func CloseOthers(files [2]*os.File, others []*os.File) {
	_ = files
	for _, other := range others {
		_ = other.Close()
	}
}
`)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for name, want := range map[string]map[int]bool{
		"CloseEach":        {0: false, 1: true},
		"CloseSlice":       {0: true},
		"CloseWhenFlagged": {0: false},
		"CloseOthers":      {0: false, 1: true},
	} {
		fact := summarize(pass, pkg.Func(name))
		for index, expected := range want {
			if got := fact.May.LoopReleased.contains(index); got != expected {
				t.Errorf("%s: LoopReleased parameter %d = %t, want %t", name, index, got, expected)
			}
		}
		if fact.MethodMask("Close") != 0 {
			t.Errorf("%s: a loop or flag must not claim Closed, got %v", name, fact.MethodMask("Close"))
		}
	}
}

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
	for name, want := range map[string]ResultMask{
		"OpenFresh":           resultMaskFor(0),
		"OpenReader":          resultMaskFor(0),
		"OpenClosedOnFailure": resultMaskFor(0),
		"OpenRegistered":      0,
		"OpenSeeked":          0,
		"OpenWithCleanup":     0,
		"OpenMaybeClosed":     0,
		"OpenView":            0,
		"OpenPlain":           resultMaskFor(0),
		"OpenForwarded":       0,
		"OpenFromFD":          resultMaskFor(0),
	} {
		fact := summarize(pass, pkg.Func(name))
		if fact.Must.OwnedResults != want {
			t.Errorf("%s: OwnedResults = %v, want %v", name, fact.Must.OwnedResults, want)
		}
	}
}

func TestReleasesEachElement(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "elementprobe", `package elementprobe
type File struct{}
func (*File) Close() error { return nil }
var kept [][]*File
func Each(files []*File) { for _, f := range files { _ = f.Close() } }
func First(files []*File) { if len(files) > 0 { _ = files[0].Close() } }
func Until(files []*File) error { for _, f := range files { if err := f.Close(); err != nil { return err } }; return nil }
func Skip(files []*File, skip func(*File) bool) { for _, f := range files { if skip(f) { continue }; _ = f.Close() } }
func Keep(files []*File) { kept = append(kept, files); for _, f := range files { _ = f.Close() } }
func Early(files []*File, early bool) { if early { return }; for _, f := range files { _ = f.Close() } }
func Twice(files []*File) { for _, f := range files { _ = f.Close() }; for _, f := range files { _ = f.Close() } }
`)
	for name, want := range map[string]bool{
		"Each": true, "First": false, "Until": false, "Skip": false, "Keep": false, "Early": false, "Twice": true,
	} {
		function := pkg.Func(name)
		if got := releasesEachElement(function, function.Params[0], "Close"); got != want {
			t.Errorf("%s: releasesEachElement = %t, want %t", name, got, want)
		}
	}
}

func TestForwardedDischargePaths(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
 type closer struct{}
 func(*closer)Close()error{return nil}
 type job struct{file,other *closer}
 func closeOne(p *closer){p.Close()}
 func Earlier(j,replacement job){saved:=j.file;j=replacement;defer closeOne(saved)}
 func Replacement(j,replacement job){j=replacement;defer closeOne(j.file)}
 func Sibling(j job){defer closeOne(j.other)}
 func Ambiguous(j,replacement job,flag bool){if flag{j=replacement};defer closeOne(j.file)}
 `)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for _, test := range []struct {
		name       string
		discharges []Discharge
	}{
		{"Earlier", []Discharge{{Parameter: 0, Method: "Close", Path: "field:0"}}},
		{"Replacement", []Discharge{{Parameter: 1, Method: "Close", Path: "field:0"}}},
		{"Sibling", []Discharge{{Parameter: 0, Method: "Close", Path: "field:1"}}},
		{"Ambiguous", nil},
	} {
		fact := summarize(pass, pkg.Func(test.name))
		if !slices.Equal(fact.Discharges, test.discharges) {
			t.Errorf("%s discharges=%+v, want %+v", test.name, fact.Discharges, test.discharges)
		}
	}
}
