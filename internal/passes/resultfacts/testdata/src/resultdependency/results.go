package resultdependency

type Failure struct{}

func (*Failure) Error() string { return "failure" }

var Sentinel error = &Failure{}

func Nil() error          { return nil }
func TypedNil() error     { return (*Failure)(nil) }
func Pair() (bool, error) { return false, nil }
func Mixed(yes bool) error {
	if yes {
		return nil
	}
	return &Failure{}
}
func Global() error         { return Sentinel }
func Deferred() (err error) { defer func() { err = &Failure{} }(); return nil }

type stored struct {
	flag bool
	err  error
}

func StoredTrue() bool {
	value := &stored{flag: true}
	return value.flag
}

func StoredTypedNil() error {
	value := &stored{err: (*Failure)(nil)}
	return value.err
}
