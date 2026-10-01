package resourcedep

import "io"

type ValueWriter struct {
	Out   io.Writer
	Level int
}

func NewValueWriter(out io.Writer) ValueWriter { return ValueWriter{Out: out} }
func (writer ValueWriter) WithLevel(level int) ValueWriter {
	writer.Level = level
	return writer
}
func (writer ValueWriter) WithOutput(out io.Writer) ValueWriter {
	writer.Out = out
	return writer
}

type ValueContext struct{ Writer ValueWriter }

func (writer ValueWriter) Context() ValueContext  { return ValueContext{Writer: writer} }
func (context ValueContext) Extract() ValueWriter { return context.Writer }
