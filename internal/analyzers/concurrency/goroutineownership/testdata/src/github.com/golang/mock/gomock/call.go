package gomock

// Call supplies the documented result-registration identity for the fixture.
type Call struct{}

func (call *Call) Return(values ...any) *Call { return call }
