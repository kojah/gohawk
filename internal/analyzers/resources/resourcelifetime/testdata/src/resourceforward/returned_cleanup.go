package resourceforward

import (
	"io"
	"resourcedep"
)

func CleanupFor(resource io.Closer) func() {
	return resourcedep.CleanupFor(resource)
}
