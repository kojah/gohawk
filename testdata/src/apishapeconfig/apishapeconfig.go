package apishapeconfig

func AllowedParameters(one, two, three, four, five int) {}

func AllowedOptional(one, two *string) {}

type AllowedMixed struct{}

func (AllowedMixed) Value()    {}
func (*AllowedMixed) Pointer() {}
