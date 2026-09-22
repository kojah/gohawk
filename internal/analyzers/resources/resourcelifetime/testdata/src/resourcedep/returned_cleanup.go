package resourcedep

import "io"

func CleanupFor(resource io.Closer) func() {
	return func() { resource.Close() }
}
