package resultdependency

func Pointer(p *int) *int       { return p }
func SliceView(p []byte) []byte { return p[1:] }

func TupleView(p []byte) (int, []byte, error) { return 0, p[1:], nil }
