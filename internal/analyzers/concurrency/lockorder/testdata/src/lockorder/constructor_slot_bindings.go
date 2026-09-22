package lockorder

import "sync"

// A fresh constructor result only weakens the class of an embedded value
// mutex. It proves neither publication safety nor freshness of pointer fields.
type constructedMutexOwner struct{ mu sync.Mutex }
type constructedMutexSlot struct{ current, other *constructedMutexOwner }

var constructedGuard, borrowedGuard, replacedGuard, conditionalGuard sync.Mutex
var wholeSlotGuard, directSlotGuard, wrongSlotGuard, conditionalSlotGuard, boxedSlotGuard sync.Mutex
var slotAddressGuard, changedReceiverGuard sync.Mutex

func makeMutexOwner() *constructedMutexOwner                               { return new(constructedMutexOwner) }
func borrowMutexOwner(owner *constructedMutexOwner) *constructedMutexOwner { return owner }
func maybeMutexOwner(owner *constructedMutexOwner, fresh bool) *constructedMutexOwner {
	if fresh {
		return new(constructedMutexOwner)
	}
	return owner
}
func takeConstructedMutex(owner *constructedMutexOwner, flag bool) {
	if flag {
		owner.mu.Lock()
		owner.mu.Unlock()
	}
}
func reverseConstructedMutex(owner *constructedMutexOwner) {
	owner.mu.Lock()
	constructedGuard.Lock()
	constructedGuard.Unlock()
	borrowedGuard.Lock()
	borrowedGuard.Unlock()
	replacedGuard.Lock()
	replacedGuard.Unlock()
	conditionalGuard.Lock()
	conditionalGuard.Unlock()
	wholeSlotGuard.Lock()
	wholeSlotGuard.Unlock()
	directSlotGuard.Lock()
	directSlotGuard.Unlock()
	wrongSlotGuard.Lock()
	wrongSlotGuard.Unlock()
	conditionalSlotGuard.Lock()
	conditionalSlotGuard.Unlock()
	boxedSlotGuard.Lock()
	boxedSlotGuard.Unlock()
	slotAddressGuard.Lock()
	slotAddressGuard.Unlock()
	changedReceiverGuard.Lock()
	changedReceiverGuard.Unlock()
	owner.mu.Unlock()
}
func freshConstructedSlot(slot *constructedMutexSlot, flag bool) {
	constructedGuard.Lock()
	defer constructedGuard.Unlock()
	slot.current = makeMutexOwner()
	takeConstructedMutex(slot.current, flag)
}
func borrowedConstructedSlot(slot *constructedMutexSlot, owner *constructedMutexOwner, flag bool) {
	borrowedGuard.Lock()
	defer borrowedGuard.Unlock()
	slot.current = borrowMutexOwner(owner)
	takeConstructedMutex(slot.current, flag) // want "contradictory lock order: .*"
}
func replaceConstructedSlot(slot *constructedMutexSlot, owner *constructedMutexOwner) {
	slot.current = owner
}
func replacedConstructedSlot(slot *constructedMutexSlot, owner *constructedMutexOwner, flag bool) {
	replacedGuard.Lock()
	defer replacedGuard.Unlock()
	slot.current = makeMutexOwner()
	replaceConstructedSlot(slot, owner)
	takeConstructedMutex(slot.current, flag) // want "contradictory lock order: .*"
}
func conditionalConstructedSlot(slot *constructedMutexSlot, owner *constructedMutexOwner, flag bool) {
	conditionalGuard.Lock()
	defer conditionalGuard.Unlock()
	slot.current = maybeMutexOwner(owner, flag)
	takeConstructedMutex(slot.current, flag) // want "contradictory lock order: .*"
}

func wholeConstructedSlotReplacement(slot *constructedMutexSlot, owner *constructedMutexOwner, flag bool) {
	wholeSlotGuard.Lock()
	defer wholeSlotGuard.Unlock()
	slot.current = makeMutexOwner()
	*slot = constructedMutexSlot{current: owner}
	takeConstructedMutex(slot.current, flag) // want "contradictory lock order: .*"
}
func directConstructedSlotReplacement(slot *constructedMutexSlot, owner *constructedMutexOwner, flag bool) {
	directSlotGuard.Lock()
	defer directSlotGuard.Unlock()
	slot.current = makeMutexOwner()
	slot.current = owner
	takeConstructedMutex(slot.current, flag) // want "contradictory lock order: .*"
}
func wrongConstructedSlot(slot *constructedMutexSlot, flag bool) {
	wrongSlotGuard.Lock()
	defer wrongSlotGuard.Unlock()
	slot.other = makeMutexOwner()
	takeConstructedMutex(slot.current, flag) // want "contradictory lock order: .*"
}
func nonDominatingConstructedSlot(slot *constructedMutexSlot, flag bool) {
	conditionalSlotGuard.Lock()
	defer conditionalSlotGuard.Unlock()
	if flag {
		slot.current = makeMutexOwner()
	}
	takeConstructedMutex(slot.current, true) // want "contradictory lock order: .*"
}
func boxedConstructedSlot(slot *constructedMutexSlot, flag bool) {
	boxedSlotGuard.Lock()
	defer boxedSlotGuard.Unlock()
	defer func() { slot.current = nil }()
	slot.current = makeMutexOwner()
	takeConstructedMutex(slot.current, flag)
}

func replaceConstructedAddress(slot **constructedMutexOwner, owner *constructedMutexOwner) {
	*slot = owner
}
func addressedConstructedSlot(slot *constructedMutexSlot, owner *constructedMutexOwner, flag bool) {
	slotAddressGuard.Lock()
	defer slotAddressGuard.Unlock()
	defer func() { slot.current = nil }()
	slot.current = makeMutexOwner()
	replaceConstructedAddress(&slot.current, owner)
	takeConstructedMutex(slot.current, flag) // want "contradictory lock order: .*"
}
func changedConstructedReceiver(slot, other *constructedMutexSlot, flag bool) {
	changedReceiverGuard.Lock()
	defer changedReceiverGuard.Unlock()
	defer func() { slot.current = nil }()
	slot.current = makeMutexOwner()
	slot = other
	takeConstructedMutex(slot.current, flag) // want "contradictory lock order: .*"
}

type pointerMutexOwner struct{ mu *sync.Mutex }
type pointerMutexSlot struct{ current *pointerMutexOwner }

var pointerSlotGuard, sharedPointerMutex sync.Mutex

func makePointerMutexOwner() *pointerMutexOwner { return &pointerMutexOwner{mu: &sharedPointerMutex} }
func takePointerMutex(owner *pointerMutexOwner, flag bool) {
	if flag {
		owner.mu.Lock()
		owner.mu.Unlock()
	}
}
func freshContainerSharedPointer(slot *pointerMutexSlot, flag bool) {
	pointerSlotGuard.Lock()
	defer pointerSlotGuard.Unlock()
	slot.current = makePointerMutexOwner()
	takePointerMutex(slot.current, flag)
}
func reverseSharedPointer(owner *pointerMutexOwner) {
	owner.mu.Lock()
	pointerSlotGuard.Lock() // want "contradictory lock order: .*"
	pointerSlotGuard.Unlock()
	owner.mu.Unlock()
}
