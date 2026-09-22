package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

const deferredEffectsFixture = `package deferred
import "sync"
func finish(mu *sync.Mutex, done chan struct{}) { mu.Unlock(); close(done) }
func finishGroup(mu *sync.Mutex, group *sync.WaitGroup) { mu.Unlock(); group.Done() }
func ordered(mu *sync.Mutex, done, last chan struct{}) {
 mu.Lock()
 defer close(last)
 defer finish(mu, done)
}
func grouped(mu *sync.Mutex, group *sync.WaitGroup) { mu.Lock(); defer finishGroup(mu, group) }
func twoGroups(first, second *sync.Mutex, done, last chan struct{}) {
 first.Lock(); second.Lock(); defer finish(first, done); defer finish(second, last)
}
func acquire(mu *sync.Mutex, done chan struct{}) { mu.Lock(); close(done) }
func receive(done chan struct{}) { <-done; close(done) }
func maybe(mu *sync.Mutex, done chan struct{}, yes bool) { if yes { finish(mu, done) } }
func deferredAcquire(mu *sync.Mutex, done chan struct{}) { defer acquire(mu, done) }
func deferredReceive(done chan struct{}) { defer receive(done) }
func deferredMaybe(mu *sync.Mutex, done chan struct{}, yes bool) { defer maybe(mu, done, yes) }
func reassigned(mu, other *sync.Mutex, done chan struct{}) {
 mu.Lock(); defer finish(mu, done); mu = other; _ = mu
}
func changedCapture(mu, other *sync.Mutex, done chan struct{}) {
 mu.Lock(); defer func() { finish(mu, done) }(); mu = other
}
`

func TestDeferredEffectGroups(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "deferred", deferredEffectsFixture)
	for _, test := range []struct {
		name       string
		kinds      []Kind
		parameters []int
	}{
		{"ordered", []Kind{Lock, Unlock, Close, Close}, []int{0, 0, 1, 2}},
		{"grouped", []Kind{Lock, Unlock, GroupDone}, []int{0, 0, 1}},
		{"twoGroups", []Kind{Lock, Lock, Unlock, Close, Unlock, Close}, []int{0, 1, 1, 3, 0, 2}},
		{"reassigned", []Kind{Lock, Unlock, Close}, []int{0, 0, 2}},
		{"deferredAcquire", nil, nil},
		{"deferredReceive", nil, nil},
		{"deferredMaybe", nil, nil},
		{"changedCapture", nil, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			result := NewEngine().Function(function, ssaflow.NewSearchBudget(2000))
			if result.Complete() != (test.kinds != nil) || len(result.Operations) != len(test.kinds) {
				t.Fatalf("unexpected summary: %+v", result)
			}
			for index, kind := range test.kinds {
				op := result.Operations[index]
				if op.Kind != kind || op.Resource.Value != function.Params[test.parameters[index]] {
					t.Errorf("operation %d: %+v", index, op)
				}
			}
		})
	}
}
