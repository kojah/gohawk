package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func TestEnclosingCallbackCompletion(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
import "testing"
type db struct{}
func (*db) Close() {}
func (*db) Use() {}
type wrapper struct { db *db }
func (w *wrapper) observe() { w.db.Use() }
func run(t *testing.T, callbacks ...func(*wrapper)) {
 for _, fn := range callbacks {
  fn := fn
  t.Run("case", func(t *testing.T) {
   database := new(db)
   t.Cleanup(func(){ database.Close() })
   fn(&wrapper{database})
  })
 }
}
func good(t *testing.T) { run(t, func(w *wrapper){ w.observe(); w.db.Use() }) }
func without(fn func(*wrapper)) { fn(&wrapper{new(db)}) }
func missing() { without(func(w *wrapper){ w.db.Use() }) }
func wrongDB(fn func(*wrapper)) { database := new(db); defer database.Close(); fn(&wrapper{new(db)}) }
func wrong() { wrongDB(func(w *wrapper){ w.db.Use() }) }
func mutated(t *testing.T) { run(t, func(w *wrapper){ w.db = new(db); w.db.Use() }) }
var saved func(*wrapper)
func escaping(fn func(*wrapper)) { database := new(db); defer database.Close(); fn(&wrapper{database}); saved = fn }
func escape() { escaping(func(w *wrapper){ w.db.Use() }) }
func deferredDB(fn func(*wrapper)) { database := new(db); defer database.Close(); fn(&wrapper{database}) }
func direct() { deferredDB(func(w *wrapper){ w.db.Use() }) }
func maybeCleanup(t *testing.T, yes bool, fn func(*wrapper)) {
 database := new(db); other := new(db); selected := database
 if yes { selected = other }
 t.Cleanup(func(){ selected.Close() }); fn(&wrapper{database})
}
func ambiguous(t *testing.T, yes bool) { maybeCleanup(t, yes, func(w *wrapper){ w.db.Use() }) }
func partialCleanup(t *testing.T, yes bool, fn func(*wrapper)) {
 database := new(db); if yes { t.Cleanup(func(){ database.Close() }) }; fn(&wrapper{database})
}
func conditional(t *testing.T, yes bool) { partialCleanup(t, yes, func(w *wrapper){ w.db.Use() }) }
func twoCalls(fn func(*wrapper)) { database := new(db); defer database.Close(); fn(&wrapper{database}); fn(&wrapper{new(db)}) }
func secondUnowned() { twoCalls(func(w *wrapper){ w.db.Use() }) }
var callbacks = map[int]func(*wrapper){}
func keepInMap(fn func(*wrapper)) { database := new(db); defer database.Close(); fn(&wrapper{database}); callbacks[0] = fn }
func mapEscape() { keepInMap(func(w *wrapper){ w.db.Use() }) }
func loopDB(t *testing.T, fn func(*wrapper)) {
 var database *db
 for i:=0; i<2; i++ { database = new(db); t.Cleanup(func(){ database.Close() }); fn(&wrapper{database}) }
}
func reusedCell(t *testing.T) { loopDB(t, func(w *wrapper){ w.db.Use() }) }
`)
	for _, test := range []struct {
		name   string
		proven bool
	}{
		{"good", true},
		{"missing", false},
		{"wrong", false},
		{"mutated", false},
		{"escape", false},
		{"direct", true},
		{"ambiguous", false},
		{"conditional", false},
		{"secondUnowned", false},
		{"mapEscape", false},
		{"reusedCell", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name).AnonFuncs[0]
			var receiver ssa.Value
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
				if ssaflow.CallName(call.Common()) == "Use" {
					receiver = ssaflow.CallReceiver(call.Common())
				}
			}
			proof := ProveEnclosingCompletion(EnclosingCompletionRequest{
				Function: fn, Value: receiver, Methods: []string{"Close"}, Budget: ssaflow.NewSearchBudget(10000),
			})
			if proof.Proven() != test.proven {
				t.Fatalf("proof = %+v, want proven %v", proof, test.proven)
			}
			limited := ProveEnclosingCompletion(EnclosingCompletionRequest{
				Function: fn, Value: receiver, Methods: []string{"Close"}, Budget: ssaflow.NewSearchBudget(1),
			})
			if limited.State != ssaflow.EvidenceUnknown || limited.Reason != ssaflow.EvidenceBudgetExhausted {
				t.Fatalf("exhausted proof = %+v", limited)
			}
		})
	}
}
