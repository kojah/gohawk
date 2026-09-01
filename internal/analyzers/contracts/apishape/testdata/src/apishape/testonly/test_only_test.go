package testonly

func ExternalTestBridge(one, two, three, four, five int) {}

type TestOnlyMixed struct{}

func (TestOnlyMixed) Value() {}

func (*TestOnlyMixed) Pointer() {}
