package apishape

func TooMany(one, two, three, four, five int) {} // want "exported API has 5 parameters" "5 adjacent parameters share type int"

func Formattingf(one, two, three int, format string, args ...any) {}

func NotFormatting(one int, two string, three bool, label string, args ...any) { // want "exported API has 5 parameters"
}

func Optional(one, two *string) {} // want "adjacent optional scalar parameters"

type Callback func(one, two, three int)

func CallbackShape(one, two, three int) {}

func registerCallback(callback Callback) {}

func registerShapes() { registerCallback(CallbackShape) }

func CompatibilityShape(_, two, three int) {}

type RequiredShape interface {
	Required(one, two, three, four, five int)
}

type RequiredImplementation struct{}

func (RequiredImplementation) Required(one, two, three, four, five int) {}

var _ RequiredShape = RequiredImplementation{}

type SimilarButUnconstrained struct{}

func (SimilarButUnconstrained) Other(one, two, three, four, five int) { // want "exported API has 5 parameters" "5 adjacent parameters share type int"
}

type Mixed struct{}

func (Mixed) Value() {}

func (*Mixed) Pointer() {} // want "type Mixed mixes pointer and value receivers"

type markerNode interface {
	Position() int
	markerNode()
}

type EmptyInterfaceMarker struct{}

func (EmptyInterfaceMarker) markerNode() {}

func (*EmptyInterfaceMarker) Position() int { return 0 }

var _ markerNode = (*EmptyInterfaceMarker)(nil)

type valueMarker interface{ valueMarker() }

type ValueInterfaceMarker struct{}

func (ValueInterfaceMarker) valueMarker() {}

func (*ValueInterfaceMarker) mutate() {}

var _ valueMarker = ValueInterfaceMarker{}

type exportedMarker interface{ Marker() }

type ExportedMarker struct{}

func (ExportedMarker) Marker() {}

func (*ExportedMarker) mutate() {} // want "type ExportedMarker mixes pointer and value receivers"

var markerSideEffect bool

type sideEffectMarker interface{ sideEffectMarker() }

type SideEffectMarker struct{}

func (SideEffectMarker) sideEffectMarker() { markerSideEffect = true }

func (*SideEffectMarker) mutate() {} // want "type SideEffectMarker mixes pointer and value receivers"

type receiverReadMarker interface{ receiverReadMarker() }

type ReceiverReadMarker struct{ state int }

func (marker ReceiverReadMarker) receiverReadMarker() { _ = marker.state }

func (*ReceiverReadMarker) mutate() {} // want "type ReceiverReadMarker mixes pointer and value receivers"

type parameterMarker interface{ parameterMarker(int) }

type ParameterMarker struct{}

func (ParameterMarker) parameterMarker(int) {}

func (*ParameterMarker) mutate() {} // want "type ParameterMarker mixes pointer and value receivers"

type resultMarker interface{ resultMarker() bool }

type ResultMarker struct{}

func (ResultMarker) resultMarker() bool { return true }

func (*ResultMarker) mutate() {} // want "type ResultMarker mixes pointer and value receivers"

type privateEmptyMethod struct{}

func (privateEmptyMethod) notAnInterfaceMarker() {}

func (*privateEmptyMethod) mutate() {} // want "type privateEmptyMethod mixes pointer and value receivers"

type substantiveMarker interface{ substantiveMarker() }

type MarkerWithSubstantiveValueMethod struct{ state int }

func (MarkerWithSubstantiveValueMethod) substantiveMarker() {}

func (marker MarkerWithSubstantiveValueMethod) stateValue() int { return marker.state }

func (*MarkerWithSubstantiveValueMethod) mutate() { // want "type MarkerWithSubstantiveValueMethod mixes pointer and value receivers"
}

type Codec struct{}

func (Codec) MarshalJSON() ([]byte, error) { return nil, nil }

func (*Codec) UnmarshalJSON([]byte) error { return nil }
