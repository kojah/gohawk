package resultforward

import "resultdependency"

func Pointer(p *int) *int       { return resultdependency.Pointer(p) }
func SliceView(p []byte) []byte { return resultdependency.SliceView(p) }

func TupleView(p []byte) (int, []byte, error) { return resultdependency.TupleView(p) }
