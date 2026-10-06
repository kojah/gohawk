package require

type TestingT interface {
	Errorf(format string, args ...any)
	FailNow()
}

func NoError(t TestingT, err error, _ ...any) {
	if err != nil {
		t.FailNow()
	}
}

func Error(t TestingT, err error, _ ...any) {
	if err == nil {
		t.FailNow()
	}
}

func NotNil(t TestingT, value any, _ ...any) {
	if value == nil {
		t.FailNow()
	}
}
