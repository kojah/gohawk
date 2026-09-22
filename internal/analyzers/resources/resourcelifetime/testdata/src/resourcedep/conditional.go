package resourcedep

import "io"

func CloseWhenReady(resource io.Closer, ready bool) bool {
	if !ready {
		return false
	}
	resource.Close()
	return true
}
