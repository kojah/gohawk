package apishape

func TooMany(one, two, three, four, five int) {} // want "exported API has 5 parameters" "5 adjacent parameters share type int"

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

type Codec struct{}

func (Codec) MarshalJSON() ([]byte, error) { return nil, nil }

func (*Codec) UnmarshalJSON([]byte) error { return nil }
