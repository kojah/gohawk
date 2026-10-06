package lifecycle

import (
	"bytes"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCallEffects(t *testing.T) {
	for _, test := range []struct {
		name  string
		body  string
		want  ssacall.CallEffect
		known bool
	}{
		{"unused", ``, 0, true},
		{"read", `_ = p.n`, ssacall.EffectRead, true},
		{"write", `p.n=1`, ssacall.EffectMutate, true},
		{"readThenRetain", `_ = p.n; saved=p`, ssacall.EffectRead | ssacall.EffectRetain, false},
		{"returnAddress", `return p`, ssacall.EffectRetain, true},
		{"nestedReader", `read(p)`, ssacall.EffectRead, true},
		{"nestedWriter", `write(p)`, ssacall.EffectMutate, true},
		{"nestedRetainer", `retain(p)`, ssacall.EffectRetain, false},
		{"asyncReader", `go read(p)`, ssacall.EffectRead | ssacall.EffectAsync, true},
		{"deferredReader", `defer read(p)`, ssacall.EffectRead, true},
		{"deferredWriter", `defer write(p)`, ssacall.EffectMutate, true},
		{"spilledClosureReader", `func(){ _=p.n }()`, ssacall.EffectRetain, false},
		{"spilledClosureWriter", `func(){ p.n=1 }()`, ssacall.EffectRetain, false},
		{"spilledRetainedClosure", `callback=func(){ _=p.n }`, ssacall.EffectRetain, false},
		{"spilledAsyncClosure", `go func(){ _=p.n }()`, ssacall.EffectRetain, false},
		{"opaque", `opaque(p)`, 0, false},
		{"dynamic", `dynamic(p)`, 0, false},
		{"recursive", `recurse(p)`, 0, false},
		{"fieldAddressRetained", `savedInt=&p.n`, ssacall.EffectRetain, false},
		{"scalarReturn", `_ = scalar(p)`, ssacall.EffectRead, true},
		{"branchEffects", `if pick { read(p) } else { write(p) }`, ssacall.EffectRead | ssacall.EffectMutate, true},
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
			proof := ssacall.NewCallEffects(proofs.NewSearchBudget(1000)).Value(fn.Params[0])
			if proof.Proven() != test.known || proof.Effects != test.want {
				t.Fatalf("effects = %+v, want known=%t effects=%v", proof, test.known, test.want)
			}
			if proof.PreservesStorage() != (test.known && test.want & ^ssacall.EffectRead == 0) {
				t.Fatalf("unexpected preservation: %+v", proof)
			}
			exhausted := ssacall.NewCallEffects(proofs.NewSearchBudget(0)).Value(fn.Params[0])
			if exhausted.Proven() || exhausted.Reason != proofs.EvidenceBudgetExhausted {
				t.Fatalf("budget exhaustion: %+v", exhausted)
			}
		})
	}
}

func TestCallEffectArgumentAndCaptureMapping(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		want ssacall.CallEffect
	}{
		{"read", `read(p)`, ssacall.EffectRead},
		{"bothArguments", `pair(p,p)`, ssacall.EffectRead | ssacall.EffectMutate},
		{"started", `go read(p)`, ssacall.EffectRead | ssacall.EffectAsync},
		{"deferred", `defer read(p)`, ssacall.EffectRead},
		{"captureCellRead", `func(){ _=p.n }()`, ssacall.EffectRead},
		{"captureCellWrite", `func(){ p=nil }()`, ssacall.EffectMutate},
		{"captureCellAsync", `go func(){ _=p.n }()`, ssacall.EffectRead | ssacall.EffectAsync},
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
					proof := ssacall.NewCallEffects(nil).Call(instruction, value)
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
