package assert

type TestingT interface {
	Errorf(format string, args ...any)
}

func NoError(_ TestingT, err error, _ ...any) bool {
	return err == nil
}
