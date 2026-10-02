package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

const resultGuardFixture = `
package ssaflowtest

type file struct{}

type failure struct{}

func (*failure) Error() string { return "failed" }

func (*file) Close() {}

func open() (*file, error) { return &file{}, nil }

func closeOnError(fail bool) (err error) {
	f, err := open()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	if fail {
		return &failure{}
	}
	return nil
}

func closeAlways() (err error) {
	f, err := open()
	if err != nil {
		return err
	}
	defer func() { f.Close() }()
	return nil
}
`

func TestResultGuards(t *testing.T) {
	pkg := buildTestSSA(t, resultGuardFixture)
	guarded := pkg.Func("closeOnError")
	target := openedFile(t, guarded)
	request := CompletionRequest{Target: target, Methods: []string{"Close"}}
	guards := ProveResultGuards(guarded, request).Guards
	if len(guards) != 1 || len(guards[0].Cells) != 1 {
		t.Fatalf("ResultGuards(closeOnError) = %+v, want one guard on the err result", guards)
	}
	states := map[string]ssaflow.EvidenceState{}
	for _, returned := range ssaflow.InstructionsOf[*ssa.Return](guarded) {
		if !ssaflow.InstructionDominates(guards[0].Defer, returned) {
			continue
		}
		value, _ := ssaflow.ValueAtReturnWithin(returned, guards[0].Cells[0], nil)
		states[value.String()] = guards[0].CompletesAtReturn(request, returned, ssaflow.ValueOutcome)
	}
	if states["nil:error"] != ssaflow.EvidenceDisproven {
		t.Errorf("the nil return: %v, want the guarded close skipped (disproven); all: %v", states["nil:error"], states)
	}
	for value, state := range states {
		if value != "nil:error" && state != ssaflow.EvidenceProven {
			t.Errorf("the %s return: %v, want the guarded close run (proven)", value, state)
		}
	}
	always := pkg.Func("closeAlways")
	if guards := ProveResultGuards(always, CompletionRequest{Target: openedFile(t, always), Methods: []string{"Close"}}).Guards; len(guards) != 0 {
		t.Errorf("ResultGuards(closeAlways) = %+v, want none: its release does not turn on the result", guards)
	}
}

func openedFile(t *testing.T, function *ssa.Function) ssa.Value {
	t.Helper()
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if extract, ok := instruction.(*ssa.Extract); ok && extract.Index == 0 {
				return extract
			}
		}
	}
	t.Fatalf("%s: no opened file", function.Name())
	return nil
}
