package lifecyclefacts

import (
	"go/token"
	"go/types"
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// contractsFor summarizes every function in the package and returns the
// cleanup contracts the pass would export, keyed by type name.
func contractsFor(t *testing.T, source string) map[string]*CleanupFact {
	t.Helper()
	pkg := buildLifecycleTestSSA(t, source)
	exported := map[string]*CleanupFact{}
	pass := &analysis.Pass{
		Pkg:              pkg.Pkg,
		ImportObjectFact: func(types.Object, analysis.Fact) bool { return false },
		ExportObjectFact: func(object types.Object, fact analysis.Fact) {
			if contract, ok := fact.(*publishedCleanup); ok {
				value := contract.Value()
				exported[object.Name()] = &value
			}
		},
	}
	summaries := Summaries{}
	// Summarize every function and method the package defines, which is what
	// the pass has in hand by the time it joins a contract.
	for _, member := range pkg.Members {
		function, ok := member.(*ssa.Function)
		if ok && len(function.Blocks) > 0 {
			summaries[function] = summarize(pass, function)
		}
		named, ok := member.(*ssa.Type)
		if !ok {
			continue
		}
		pointer := types.NewPointer(named.Type())
		for selection := range types.NewMethodSet(pointer).Methods() {
			method := pkg.Prog.LookupMethod(pointer, pkg.Pkg, selection.Obj().Name())
			if method != nil && len(method.Blocks) > 0 {
				summaries[method] = summarize(pass, method)
			}
		}
	}
	exportCleanupContracts(pass, summaries)
	return exported
}

// A type whose constructor takes ownership of a resource and whose method
// releases it carries a contract, even though the method is not named Close.
func TestCleanupContractProvedFromRelease(t *testing.T) {
	contracts := contractsFor(t, `
package lifecyclefactstest

import "os"

type Scheduler struct {
	file *os.File
	name   string
}

func NewScheduler(name string) (*Scheduler, error) {
	file, err := os.CreateTemp("", name)
	if err != nil { return nil, err }
	return &Scheduler{file: file, name: name}, nil
}

func (s *Scheduler) Stop() { _ = s.file.Close() }
`)
	contract, ok := contracts["Scheduler"]
	if !ok {
		t.Fatalf("Scheduler has no cleanup contract, want one")
	}
	if len(contract.Methods) != 1 || contract.Methods[0] != "Stop" {
		t.Errorf("Scheduler methods = %v, want [Stop]", contract.Methods)
	}
	if !contract.Owned.contains(0) || contract.Released&contract.Owned != contract.Owned {
		t.Errorf("Scheduler owned=%#x released=%#x, want the release to cover the owned field",
			uint64(contract.Owned), uint64(contract.Released))
	}
}

// The method name is a label on evidence, never the evidence. A Stop that
// releases nothing proves nothing.
func TestCleanupContractRejectsMisleadingName(t *testing.T) {
	contracts := contractsFor(t, `
package lifecyclefactstest

import "os"

type Pinger struct {
	file *os.File
	count  int
}

func NewPinger() (*Pinger, error) {
	file, err := os.CreateTemp("", "pinger")
	if err != nil { return nil, err }
	return &Pinger{file: file}, nil
}

func (p *Pinger) Stop() { p.count = 0 }
`)
	if contract, ok := contracts["Pinger"]; ok {
		t.Errorf("Pinger has contract %v, want none: its Stop releases nothing", contract.Methods)
	}
}

// Owning a resource is not a contract. Without a releasing method the caller
// is owed nothing it can act on.
func TestCleanupContractNeedsAReleaser(t *testing.T) {
	contracts := contractsFor(t, `
package lifecyclefactstest

import "os"

type Probe struct{ file *os.File }

func NewProbe() (*Probe, error) {
	file, err := os.CreateTemp("", "probe")
	if err != nil { return nil, err }
	return &Probe{file: file}, nil
}
`)
	if _, ok := contracts["Probe"]; ok {
		t.Errorf("Probe has a contract, want none: nothing releases its file")
	}
}

// A partial release is not a contract: reading it as one would discharge an
// obligation that still stands.
func TestCleanupContractRejectsPartialRelease(t *testing.T) {
	contracts := contractsFor(t, `
package lifecyclefactstest

import "os"

type Pair struct {
	other *os.File
	file   *os.File
}

func NewPair(path string) (*Pair, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	other, err := os.CreateTemp("", "pair")
	if err != nil { file.Close(); return nil, err }
	return &Pair{other: other, file: file}, nil
}

func (p *Pair) Stop() { p.other.Close() }
`)
	if contract, ok := contracts["Pair"]; ok {
		t.Errorf("Pair has contract %v, want none: Stop leaves the file open", contract.Methods)
	}
}

// A borrowed resource is not owned, so a wrapper carries no contract of its
// own however it is released.
func TestCleanupContractIgnoresBorrowedResource(t *testing.T) {
	contracts := contractsFor(t, `
package lifecyclefactstest

import "os"

type Wrapper struct{ file *os.File }

func NewWrapper(file *os.File) *Wrapper { return &Wrapper{file: file} }

func (w *Wrapper) Stop() { w.file.Close() }
`)
	if _, ok := contracts["Wrapper"]; ok {
		t.Errorf("Wrapper has a contract, want none: it never acquired the file")
	}
}

// A Stop method on a timer-only wrapper does not make GC-managed timers an
// obligation again through the inferred-owner contract.
func TestCleanupContractIgnoresChannelTimers(t *testing.T) {
	contracts := contractsFor(t, `
package lifecyclefactstest
import "time"
type Clock struct { timer *time.Timer; ticker *time.Ticker }
func NewClock() *Clock {
	return &Clock{timer: time.NewTimer(time.Hour), ticker: time.NewTicker(time.Second)}
}
func (c *Clock) Stop() { c.timer.Stop(); c.ticker.Stop() }
`)
	if contract, ok := contracts["Clock"]; ok {
		t.Errorf("Clock has cleanup contract %v, want none for channel timers", contract.Methods)
	}
}

// A cleanup of a replacement aggregate must never settle the original
// parameter's field. A saved field value keeps its own earlier snapshot.
func TestSpillReplacementCleanupContracts(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
 type closer struct{}
 func(*closer)Close()error{return nil}
 type job struct {file *closer}
 func Replacement(j,other job){j=other;j.file.Close()}
 func Earlier(j,other job){saved:=j.file;j=other;saved.Close()}
 func WrappedEarlier(j,other job){saved:=j.file;j=other;var c interface{Close()error}=saved;c.Close()}
 func RestoredAfterRead(j,other job){original:=j;j=other;saved:=j.file;j=original;saved.Close()}
 func Ambiguous(j,other job,flag bool){if flag{j=other};j.file.Close()}
 func Agreeing(j job,flag bool){original:=j;if flag{j=original};j.file.Close()}
 `)
	pass := &analysis.Pass{ImportObjectFact: func(types.Object, analysis.Fact) bool { return false }}
	for _, test := range []struct {
		name       string
		parameters []int
	}{
		{"Replacement", []int{1}},
		{"Earlier", []int{0}},
		{"WrappedEarlier", []int{0}},
		{"RestoredAfterRead", []int{1}},
		{"Ambiguous", nil},
		{"Agreeing", []int{0}},
	} {
		fact := summarize(pass, pkg.Func(test.name))
		var got []int
		for _, discharge := range fact.Discharges {
			if discharge.Method == "Close" && discharge.Path == "field:0" {
				got = append(got, discharge.Parameter)
			}
		}
		if !slices.Equal(got, test.parameters) || fact.MethodMask("Close") != 0 {
			t.Errorf("%s fields=%v, whole mask=%v, want%v", test.name, got, fact.MethodMask("Close"), test.parameters)
		}
	}
}

func TestFreshResourceAcquisitionContracts(t *testing.T) {
	contracts := contractsFor(t, `
package lifecyclefactstest
import "os"
type Lazy struct { file *os.File }
func NewLazy() *Lazy { return &Lazy{} }
func (l *Lazy) Close() error {
	if l.file != nil { return l.file.Close() }
	return nil
}
type Holder struct { lazy *Lazy }
func NewHolder() *Holder { return &Holder{lazy:NewLazy()} }
func (h *Holder) Close() error { return h.lazy.Close() }
type FileOwner struct { file *os.File }
func NewFileOwner(path string) (*FileOwner, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	return &FileOwner{file:f}, nil
}
func (o *FileOwner) Close() error { return o.file.Close() }
type Manager struct { handles map[string]*os.File }
type Shared struct { file *os.File }
func (m *Manager) Open(path string) (*Shared, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	m.handles[path] = f
	return &Shared{file:f}, nil
}
func (o *Shared) Close() error { return o.file.Close() }
type Local struct { file *os.File }
func LocalOpen(path string) (*Local, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	local := map[string]*os.File{path:f}
	_ = local
	return &Local{file:f}, nil
}
func (o *Local) Close() error { return o.file.Close() }
`)
	for name, wantOwned := range map[string]bool{"Holder": false, "FileOwner": true, "Shared": false, "Local": true} {
		got := contracts[name]
		if owned := got != nil && got.Owned.contains(0); owned != wantOwned {
			t.Errorf("%s ownership = %v (%v), want %v", name, owned, got, wantOwned)
		}
	}
}

func resourceTestType(packagePath, name string) *types.Named {
	pkg := types.NewPackage(packagePath, "fixture")
	object := types.NewTypeName(token.NoPos, pkg, name, nil)
	return types.NewNamed(object, types.NewStruct(nil, nil), nil)
}

func TestResourceCleanupIdentityAndOwnership(t *testing.T) {
	for _, test := range []struct {
		packagePath string
		name        string
		methods     []string
	}{
		{"os", "File", []string{"Close"}},
		{"database/sql", "Tx", []string{"Commit", "Rollback"}},
		{"database/sql", "Rows", []string{"Close"}},
		{"database/sql", "Stmt", []string{"Close"}},
		{"net/http", "Response", []string{"Close"}},
		{"compress/gzip", "Reader", []string{"Close"}},
		{"compress/gzip", "Writer", []string{"Close"}},
		{"compress/zlib", "Writer", []string{"Close"}},
	} {
		named := resourceTestType(test.packagePath, test.name)
		for _, value := range []types.Type{named, types.NewPointer(named)} {
			methods, known := ResourceCleanup(value)
			if !known || !slices.Equal(methods, test.methods) {
				t.Fatalf("%s.%s cleanup %v, known %t", test.packagePath, test.name, methods, known)
			}
			if pkg, known := resourcePackage(value); !known || pkg != test.packagePath {
				t.Fatalf("%s.%s package %q, known %t", test.packagePath, test.name, pkg, known)
			}
			for index := range methods {
				methods[index] = "Changed"
			}
			methods = append(methods, "Added")
			fresh, _ := ResourceCleanup(value)
			if !slices.Equal(fresh, test.methods) {
				t.Fatalf("caller mutation %v changed later cleanup: %v", methods, fresh)
			}
		}
	}
	for _, value := range []types.Type{
		nil, types.Typ[types.Int], resourceTestType("example.com/user", "File"),
		resourceTestType("os", "Other"), resourceTestType("time", "Timer"),
		types.NewPointer(types.NewPointer(resourceTestType("os", "File"))),
	} {
		if methods, known := ResourceCleanup(value); known || methods != nil {
			t.Fatalf("unknown type %v gained cleanup %v", value, methods)
		}
		if pkg, known := resourcePackage(value); known || pkg != "" {
			t.Fatalf("unknown type %v gained package %q", value, pkg)
		}
	}
}

func BenchmarkResourceVocabulary(b *testing.B) {
	for _, test := range []struct {
		name  string
		value types.Type
	}{
		{"basic", types.Typ[types.Int]},
		{"unknown_named", resourceTestType("example.com/user", "File")},
		{"file", types.NewPointer(resourceTestType("os", "File"))},
		{"transaction", types.NewPointer(resourceTestType("database/sql", "Tx"))},
	} {
		b.Run(test.name, func(b *testing.B) {
			b.Run("cleanup", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_, _ = ResourceCleanup(test.value)
				}
			})
			b.Run("package", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					_, _ = resourcePackage(test.value)
				}
			})
		})
	}
}

func borrowedResourceValues(tb testing.TB) []ssa.Value {
	tb.Helper()
	pkg := ssaflowtest.BuildPackage(tb, "os", `package os
type File struct{}
func parameter(file *File) *File { return file }
func load(cell **File) *File { return *cell }
func zero() *File { return new(File) }
`)
	return []ssa.Value{
		pkg.Func("parameter").Params[0],
		ssaflow.InstructionsOf[*ssa.UnOp](pkg.Func("load"))[0],
		ssaflow.InstructionsOf[*ssa.Alloc](pkg.Func("zero"))[0],
	}
}

func TestResourceTypesDoNotEstablishAcquisition(t *testing.T) {
	for _, value := range borrowedResourceValues(t) {
		if _, known := ResourceCleanup(value.Type()); !known {
			t.Fatalf("fixture lost known resource type: %v", value)
		}
		if acquiredResource(nil, value) {
			t.Fatalf("non-call value gained acquisition evidence: %T %v", value, value)
		}
	}
}

func BenchmarkNonCallResourceAcquisition(b *testing.B) {
	values := borrowedResourceValues(b)
	for _, value := range values {
		b.Run(value.Name(), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if acquiredResource(nil, value) {
					b.Fatal("non-call value gained acquisition evidence")
				}
			}
		})
	}
}
