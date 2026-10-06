package resourcemodel

// Obligation is one path's state for an acquired resource. It records
// acquisition separately from discharge and uncertainty: an opaque handoff
// cannot prove cleanup, while a known release or transfer settles the duty.
// The value is immutable and comparable so a bounded flow walk can key states.
type Obligation struct {
	active  bool
	settled bool
	unknown bool
}

// Acquired starts a live resource obligation.
func Acquired() Obligation { return Obligation{active: true} }

// Active reports whether this path actually acquired the resource.
func (state Obligation) Active() bool { return state.active }

// Settled reports whether cleanup or ownership transfer was proven.
func (state Obligation) Settled() bool { return state.settled }

// Unknown reports whether an opaque operation prevents a leak proof.
func (state Obligation) Unknown() bool { return state.unknown }

// Absent removes the obligation on a path that did not acquire the resource
// or cannot return normally.
func (state Obligation) Absent() Obligation {
	state.active = false
	return state
}

// Discharged records a proven cleanup or ownership transfer on this path.
func (state Obligation) Discharged() Obligation {
	state.settled = true
	return state
}

// Uncertain records an opaque use without pretending that it discharged the
// obligation. Later exact evidence cannot make this earlier opacity vanish.
func (state Obligation) Uncertain() Obligation {
	state.unknown = true
	return state
}

// Unsettled reports a live obligation with neither discharge nor opaque use.
// An analyzer must still establish a feasible return and no returned owner
// before treating this state as a violation.
func (state Obligation) Unsettled() bool {
	return state.active && !state.settled && !state.unknown
}
