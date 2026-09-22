package resourcedep

type resultFailure struct{}

func (*resultFailure) Error() string { return "failure" }

var resultSentinel error = &resultFailure{}

func NilResult() error      { return nil }
func TypedNilResult() error { return (*resultFailure)(nil) }
func TrueResult() bool      { return true }
func FalseResult() bool     { return false }
func MixedResult(yes bool) error {
	if yes {
		return nil
	}
	return &resultFailure{}
}
func MutableResult() error        { return resultSentinel }
func DeferredResult() (err error) { defer func() { err = &resultFailure{} }(); return nil }
