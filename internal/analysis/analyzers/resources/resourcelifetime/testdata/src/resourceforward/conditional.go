package resourceforward

import (
	"io"
	"resourcedep"
)

func CloseWhenReady(resource io.Closer, ready bool) bool {
	return resourcedep.CloseWhenReady(resource, ready)
}
