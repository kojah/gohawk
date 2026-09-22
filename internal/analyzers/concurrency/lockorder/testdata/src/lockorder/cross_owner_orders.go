package lockorder

import "sync"

// Declaration cycles require a same-owner relation when two fields belong to
// the same owner type. Different SSA participants are not proved disjoint;
// cross-function cycles between such participants are a deliberate gap.
// Exact opposing orders inside one function remain observable.
type participantLocks struct{ first, second sync.Mutex }

func acrossParticipants(first, second *participantLocks) {
	first.first.Lock()
	second.second.Lock()
	second.second.Unlock()
	first.first.Unlock()
}

func withinParticipant(owner *participantLocks) {
	owner.second.Lock()
	owner.first.Lock()
	owner.first.Unlock()
	owner.second.Unlock()
}

func exactOpposingParticipants(first, second *participantLocks, reverse bool) {
	if reverse {
		second.second.Lock()
		first.first.Lock()
		first.first.Unlock()
		second.second.Unlock()
		return
	}
	first.first.Lock()
	second.second.Lock() // want "contradictory lock order: .*"
	second.second.Unlock()
	first.first.Unlock()
}

type sameParticipantLocks struct{ first, second sync.Mutex }

func sameParticipantForward(owner *sameParticipantLocks) {
	owner.first.Lock()
	owner.second.Lock()
	owner.second.Unlock()
	owner.first.Unlock()
}

func sameParticipantReverse(owner *sameParticipantLocks) {
	owner.second.Lock()
	owner.first.Lock() // want "contradictory lock order: .*"
	owner.first.Unlock()
	owner.second.Unlock()
}

type helperParticipantLocks struct{ first, second sync.Mutex }

func helperParticipantSecond(owner *helperParticipantLocks, flag bool) {
	if flag {
		owner.second.Lock()
		owner.second.Unlock()
	}
}

func acrossHelperParticipants(first, second *helperParticipantLocks, flag bool) {
	first.first.Lock()
	helperParticipantSecond(second, flag)
	first.first.Unlock()
}

func withinHelperParticipant(owner *helperParticipantLocks) {
	owner.second.Lock()
	owner.first.Lock()
	owner.first.Unlock()
	owner.second.Unlock()
}

// A callee-local loaded participant is not a bound second caller root. Its
// existing class hazard is retained until an actual binding supplies evidence.
type unboundParticipantLocks struct{ first, second sync.Mutex }
type participantHolder struct{ owner *unboundParticipantLocks }

func unboundParticipantSecond(holder *participantHolder, flag bool) {
	if flag {
		holder.owner.second.Lock()
		holder.owner.second.Unlock()
	}
}

func unboundParticipantForward(owner *unboundParticipantLocks, holder *participantHolder, flag bool) {
	owner.first.Lock()
	unboundParticipantSecond(holder, flag)
	owner.first.Unlock()
}

func unboundParticipantReverse(owner *unboundParticipantLocks) {
	owner.second.Lock()
	owner.first.Lock() // want "contradictory lock order: .*"
	owner.first.Unlock()
	owner.second.Unlock()
}
