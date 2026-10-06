package processdep

// These result contracts deliberately say nothing about process ownership.
func SuccessfulPreparation() error    { return nil }
func Enabled() bool                   { return true }
func PossibleFailure(err error) error { return err }

type resultFailure struct{}

func (*resultFailure) Error() string { return "failure" }
func BoxedNilFailure() error {
	var err *resultFailure
	return err
}
