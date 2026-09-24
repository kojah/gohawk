package nilargument

import "addressdep"

// A helper can return the address of an embedded value. Its zero contents
// do not make the address nil. The nil-dereference control is Length(nil)
// in nilargument.go.

type embeddedIterator struct{ index int }
type iteratorOwner struct {
	iterator embeddedIterator
}

func (owner *iteratorOwner) Embedded() *embeddedIterator { return &owner.iterator }
func iteratorIndex(iterator *embeddedIterator) int       { return iterator.index }

func returnedEmbeddedAddress() int {
	var owner iteratorOwner
	return iteratorIndex(owner.Embedded())
}

func importedEmbeddedAddress() int {
	var owner addressdep.Owner
	return addressdep.Use(addressdep.Embedded(&owner))
}
