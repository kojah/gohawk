package lifecycle

import (
	"bytes"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCallEffects(t *testing.T) {
	for _, test := range []struct {
		name  string
		body  string
		want  ssaflow.CallEffect
		known bool
	}{
		{"unused", ``, 0, true},
		{"read", `_ = p.n`, ssaflow.EffectRead, true},
		{"write", `p.n=1`, ssaflow.EffectMutate, true},
		{"readThenRetain", `_ = p.n; saved=p`, ssaflow.EffectRead | ssaflow.EffectRetain, false},
		{"returnAddress", `return p`, ssaflow.EffectRetain, true},
		{"nestedReader", `read(p)`, ssaflow.EffectRead, true},
		{"nestedWriter", `write(p)`, ssaflow.EffectMutate, true},
		{"nestedRetainer", `retain(p)`, ssaflow.EffectRetain, false},
		{"asyncReader", `go read(p)`, ssaflow.EffectRead | ssaflow.EffectAsync, true},
		{"deferredReader", `defer read(p)`, ssaflow.EffectRead, true},
		{"deferredWriter", `defer write(p)`, ssaflow.EffectMutate, true},
		{"spilledClosureReader", `func(){ _=p.n }()`, ssaflow.EffectRetain, false},
		{"spilledClosureWriter", `func(){ p.n=1 }()`, ssaflow.EffectRetain, false},
		{"spilledRetainedClosure", `callback=func(){ _=p.n }`, ssaflow.EffectRetain, false},
		{"spilledAsyncClosure", `go func(){ _=p.n }()`, ssaflow.EffectRetain, false},
		{"opaque", `opaque(p)`, 0, false},
		{"dynamic", `dynamic(p)`, 0, false},
		{"recursive", `recurse(p)`, 0, false},
		{"fieldAddressRetained", `savedInt=&p.n`, ssaflow.EffectRetain, false},
		{"scalarReturn", `_ = scalar(p)`, ssaflow.EffectRead, true},
		{"branchEffects", `if pick { read(p) } else { write(p) }`, ssaflow.EffectRead | ssaflow.EffectMutate, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "effectprobe", `package effectprobe
type owner struct { n int }
var saved *owner
var savedInt *int
var callback func()
var dynamic func(*owner)
func opaque(*owner)
func read(p *owner) { _=p.n }
func write(p *owner) { p.n=1 }
func retain(p *owner) { saved=p }
func recurse(p *owner) { recurse(p) }
func scalar(p *owner) int { return p.n }
func probe(p *owner,pick bool) *owner { `+test.body+`;return nil }
`)
			fn := pkg.Func("probe")
			var dump bytes.Buffer
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			proof := ssaflow.NewCallEffects(ssaflow.NewSearchBudget(1000)).Value(fn.Params[0])
			if proof.Proven() != test.known || proof.Effects != test.want {
				t.Fatalf("effects = %+v, want known=%t effects=%v", proof, test.known, test.want)
			}
			if proof.PreservesStorage() != (test.known && test.want & ^ssaflow.EffectRead == 0) {
				t.Fatalf("unexpected preservation: %+v", proof)
			}
			exhausted := ssaflow.NewCallEffects(ssaflow.NewSearchBudget(0)).Value(fn.Params[0])
			if exhausted.Proven() || exhausted.Reason != ssaflow.EvidenceBudgetExhausted {
				t.Fatalf("budget exhaustion: %+v", exhausted)
			}
		})
	}
}

func TestCallEffectArgumentAndCaptureMapping(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want ssaflow.CallEffect
	}{
		{"read", `read(p)`, ssaflow.EffectRead},
		{"bothArguments", `pair(p,p)`, ssaflow.EffectRead | ssaflow.EffectMutate},
		{"started", `go read(p)`, ssaflow.EffectRead | ssaflow.EffectAsync},
		{"deferred", `defer read(p)`, ssaflow.EffectRead},
		{"captureCellRead", `func(){ _=p.n }()`, ssaflow.EffectRead},
		{"captureCellWrite", `func(){ p=nil }()`, ssaflow.EffectMutate},
		{"captureCellAsync", `go func(){ _=p.n }()`, ssaflow.EffectRead | ssaflow.EffectAsync},
	} {
		t.Run(test.name, func(t *testing.T) {
			pkg := ssaflowtest.BuildPackage(t, "effectprobe", `package effectprobe
type owner struct { n int }
func read(p *owner) { _=p.n }
func pair(p,q *owner) { _=p.n; q.n=1 }
func probe(p *owner) { `+test.body+` }
`)
			fn := pkg.Func("probe")
			for _, block := range fn.Blocks {
				for _, instruction := range block.Instrs {
					common := ssaflow.InstructionCall(instruction)
					if common == nil {
						continue
					}
					var value ssa.Value = fn.Params[0]
					if closure, ok := common.Value.(*ssa.MakeClosure); ok {
						value = closure.Bindings[0]
					}
					proof := ssaflow.NewCallEffects(nil).Call(instruction, value)
					if !proof.Proven() || proof.Effects != test.want {
						t.Fatalf("call effects = %+v, want %v", proof, test.want)
					}
					return
				}
			}
			t.Fatal("missing call")
		})
	}
}
