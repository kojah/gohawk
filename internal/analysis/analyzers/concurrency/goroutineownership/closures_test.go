package goroutineownership

import (
	"bytes"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"github.com/kojah/gohawk/internal/testsupport/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

func TestClosureChoiceCaptures(t *testing.T) {
	for _, test := range []struct {
		name, branches, launch string
		want                   bool
	}{
		{"mixed", "if flag {worker=func(){<-target}} else {worker=func(){<-other}}", "go worker()", true},
		{"unrelated", "if flag {worker=func(){<-other}} else {worker=func(){println(1)}}", "go worker()", false},
		{"loop", "if flag {worker=func(){<-target}} else {worker=func(){<-other}}", "for range count {go worker()}", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := `package choiceprobe
func subject(flag bool,count int){
 target:=make(chan int);other:=make(chan int)
 go func(){close(target)}()
 var worker func()
` + test.branches + ";" + test.launch + "}"
			pkg := ssaflowtest.BuildPackage(t, "choiceprobe", source)
			fn := pkg.Func("subject")
			target := ssaflow.InstructionsOf[*ssa.MakeChan](fn)[0]
			calls := ssaflow.InstructionsOf[*ssa.Go](fn)
			candidate := &spawnAnalysis{
				function: fn, spawn: calls[0], signals: []ssa.Value{target},
				tracked: []trackedValue{{value: target, kind: trackedSignal}},
				pool:    proofs.NewSearchBudget(spawnPoolBudget),
			}
			if got := candidate.closureConsumes(calls[1].Common().Value); got != test.want {
				t.Errorf("captures=%v want %v", got, test.want)
			}
			if got := candidate.otherWorkerConsumesSignal(); got != test.want {
				t.Errorf("participant=%v want %v", got, test.want)
			}
		})
	}
}

func TestClosureChoiceCaptureCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "choicecut", `package choicecut
 func subject(flag bool){target:=make(chan int);other:=make(chan int);var worker func()
 if flag {worker=func(){<-target}} else {worker=func(){<-other}}
 go worker()}`)
	fn := pkg.Func("subject")
	target := ssaflow.InstructionsOf[*ssa.MakeChan](fn)[0]
	value := ssaflow.InstructionsOf[*ssa.Go](fn)[0].Common().Value
	candidate := &spawnAnalysis{tracked: []trackedValue{{value: target, kind: trackedSignal}}, pool: proofs.NewSearchBudget(1)}
	if !candidate.closureConsumes(value) || !candidate.pool.Exhausted() {
		t.Fatal("cutoff established absent captures")
	}
	candidate.pool = proofs.NewSearchBudget(spawnPoolBudget)
	if !candidate.closureConsumes(value) || candidate.pool.Exhausted() {
		t.Fatal("fresh query lost possible capture")
	}
}

func TestClosureChoiceFixtures(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "closurechoices")
}

func TestCallbackTargetForms(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "callbackforms", `package callbackforms
 type Fn func()
 type Other func()
 var sink int
 func worker() {}
 func direct() Fn { return worker }
 func wrapped() Other { return Other(Fn(worker)) }
 func captured(value int) Other { return Other(Fn(func() { sink = value })) }
 func unknown(callback Fn) Fn { return callback }
 func asserted(value any) Fn { return value.(Fn) }
 func merged(first, second Fn, choose bool) Fn {
     result := first
     if choose { result = second }
     return result
 }
`)
	for _, name := range []string{"direct", "wrapped", "captured", "unknown", "asserted", "merged"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			var dump bytes.Buffer
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			value := ssaflow.InstructionsOf[*ssa.Return](function)[0].Results[0]
			got, closure := callbackTarget(value)
			switch name {
			case "direct", "wrapped":
				if got != pkg.Func("worker") || closure != nil {
					t.Fatal("static callback target changed")
				}
			case "captured":
				closures := ssaflow.InstructionsOf[*ssa.MakeClosure](function)
				if len(closures) != 1 || closure != closures[0] || got != closure.Fn {
					t.Fatal("callback capture binding was lost")
				}
			default:
				if got != nil || closure != nil {
					t.Fatal("opaque callback supplied an exact target")
				}
			}
		})
	}
}
