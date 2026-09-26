package lifecyclefacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
)

func TestReleasedUseProofs(t *testing.T) {
	pkg := buildLifecycleTestSSA(t, `package lifecyclefactstest
type rows struct{}
func (*rows) Close() error { return nil }
func (*rows) Next() bool { return false }
func (*rows) Scan() error { return nil }
func (*rows) Err() error { return nil }
func inspect(*rows) {}

func Loop(r *rows) {
	r.Close()
	for r.Next() {
		r.Scan()
	}
}
func Internal(r *rows, n int) {
	if n > 0 {
		r.Close()
	}
	r.Next()
}
func Both(r *rows, first, second bool) {
	if first {
		if second {
			r.Close()
		}
	}
	r.Next()
}
func Touched(r *rows) {
	r.Close()
	inspect(r)
	r.Next()
}
func Before(r *rows) {
	r.Next()
	r.Close()
}
`)
	unconditional := ssaflow.CallCondition{}
	both := ssaflow.CallCondition{Arguments: ssaflow.ArgumentConstants{Bound: 0b110, Values: 0b110}}
	for _, test := range []struct {
		name string
		want []ReleasedUse
	}{
		// Work later in the loop body is not between the release and the
		// first Next, which the Close dominates. Scan is not claimed: Next
		// touches the parameter between the Close and it.
		{"Loop", []ReleasedUse{{Condition: unconditional, Release: "Close", Use: "Next"}}},
		// A release decided by the function's own data does not dominate.
		{"Internal", nil},
		{"Both", []ReleasedUse{{Condition: both, Release: "Close", Use: "Next"}}},
		{"Touched", nil},
		{"Before", nil},
	} {
		got := releasedUses(releasedUseProofs(pkg.Func(test.name)))
		if len(got) != len(test.want) {
			t.Errorf("%s: released uses = %+v, want %+v", test.name, got, test.want)
			continue
		}
		for index := range got {
			if got[index] != test.want[index] {
				t.Errorf("%s: released use %d = %+v, want %+v", test.name, index, got[index], test.want[index])
			}
		}
	}
}
