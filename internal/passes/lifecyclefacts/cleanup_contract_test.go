package lifecyclefacts

import (
	"go/types"
	"testing"

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
			if contract, ok := fact.(*CleanupFact); ok {
				exported[object.Name()] = contract
			}
		},
	}
	summaries := Summaries{}
	// Summarize every function and method the package defines, which is what
	// the pass has in hand by the time it joins a contract.
	for _, member := range pkg.Members {
		function, ok := member.(*ssa.Function)
		if ok && len(function.Blocks) > 0 {
			summaries[function] = summarize(pass, newRetentionCache(), function)
		}
		named, ok := member.(*ssa.Type)
		if !ok {
			continue
		}
		pointer := types.NewPointer(named.Type())
		for selection := range types.NewMethodSet(pointer).Methods() {
			method := pkg.Prog.LookupMethod(pointer, pkg.Pkg, selection.Obj().Name())
			if method != nil && len(method.Blocks) > 0 {
				summaries[method] = summarize(pass, newRetentionCache(), method)
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

import "time"

type Scheduler struct {
	ticker *time.Ticker
	name   string
}

func NewScheduler(name string) *Scheduler {
	return &Scheduler{ticker: time.NewTicker(time.Second), name: name}
}

func (s *Scheduler) Stop() { s.ticker.Stop() }
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

import "time"

type Pinger struct {
	ticker *time.Ticker
	count  int
}

func NewPinger() *Pinger { return &Pinger{ticker: time.NewTicker(time.Second)} }

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

import "time"

type Probe struct{ ticker *time.Ticker }

func NewProbe() *Probe { return &Probe{ticker: time.NewTicker(time.Second)} }
`)
	if _, ok := contracts["Probe"]; ok {
		t.Errorf("Probe has a contract, want none: nothing releases its ticker")
	}
}

// A partial release is not a contract: reading it as one would discharge an
// obligation that still stands.
func TestCleanupContractRejectsPartialRelease(t *testing.T) {
	contracts := contractsFor(t, `
package lifecyclefactstest

import (
	"os"
	"time"
)

type Pair struct {
	ticker *time.Ticker
	file   *os.File
}

func NewPair(path string) (*Pair, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &Pair{ticker: time.NewTicker(time.Second), file: file}, nil
}

func (p *Pair) Stop() { p.ticker.Stop() }
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

import "time"

type Wrapper struct{ ticker *time.Ticker }

func NewWrapper(ticker *time.Ticker) *Wrapper { return &Wrapper{ticker: ticker} }

func (w *Wrapper) Stop() { w.ticker.Stop() }
`)
	if _, ok := contracts["Wrapper"]; ok {
		t.Errorf("Wrapper has a contract, want none: it never acquired the ticker")
	}
}
