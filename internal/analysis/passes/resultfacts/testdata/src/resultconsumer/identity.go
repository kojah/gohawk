package resultconsumer

import "resultforward"

func Pointer(p *int) *int       { return resultforward.Pointer(p) }
func SliceView(p []byte) []byte { return resultforward.SliceView(p) }

func TupleView(p []byte) (int, []byte, error) { return resultforward.TupleView(p) }
