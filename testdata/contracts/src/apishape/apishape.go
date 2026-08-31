package apishape

func TooMany(one, two, three, four, five int) {} // want "exported API has 5 parameters" "5 adjacent parameters share type int"

func Optional(one, two *string) {} // want "adjacent optional scalar parameters"

type Mixed struct{}

func (Mixed) Value() {}

func (*Mixed) Pointer() {} // want "type Mixed mixes pointer and value receivers"

type Codec struct{}

func (Codec) MarshalJSON() ([]byte, error) { return nil, nil }

func (*Codec) UnmarshalJSON([]byte) error { return nil }
