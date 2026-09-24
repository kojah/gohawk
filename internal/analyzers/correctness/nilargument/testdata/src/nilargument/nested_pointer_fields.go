package nilargument

// A field selected through a pointer belongs to the pointee, not to a
// flattened slot under the outer local. Unknown constructor contents cannot
// establish that inner.prog is nil. The exact nil-versus-unknown control is
// tested at the heap query, where nested requirement publication is separate.
type nestedInner struct{ prog *node }
type nestedOuter struct{ inner *nestedInner }

func unmodeledInner() *nestedInner
func nestedValue(outer *nestedOuter) int { return outer.inner.prog.value }

func unknownNestedField() int {
	return nestedValue(&nestedOuter{inner: unmodeledInner()})
}
