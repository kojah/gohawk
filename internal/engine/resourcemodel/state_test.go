package resourcemodel

import "testing"

func TestObligationTransitions(t *testing.T) {
	for _, test := range []struct {
		name      string
		state     Obligation
		active    bool
		settled   bool
		unknown   bool
		unsettled bool
	}{
		{"acquired", Acquired(), true, false, false, true},
		{"absent", Acquired().Absent(), false, false, false, false},
		{"discharged", Acquired().Discharged(), true, true, false, false},
		{"opaque", Acquired().Uncertain(), true, false, true, false},
		{"opaque-then-discharged", Acquired().Uncertain().Discharged(), true, true, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.state.Active() != test.active || test.state.Settled() != test.settled ||
				test.state.Unknown() != test.unknown || test.state.Unsettled() != test.unsettled {
				t.Errorf("state = %+v, want active=%t settled=%t unknown=%t unsettled=%t",
					test.state, test.active, test.settled, test.unknown, test.unsettled)
			}
		})
	}
}
