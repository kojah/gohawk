package resourcedep

import "io"

func StartWriter(writer io.Writer) {
	go func() { _, _ = writer.Write(nil) }()
}

func StartOtherWriter(writer, other io.Writer) {
	_, _ = writer.Write(nil)
	go func() { _, _ = other.Write(nil) }()
}
