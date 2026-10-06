package resourcedep

import "io"

type ValueWriter struct {
	Out   io.Writer
	Level int
	Hooks []func()
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

func (writer ValueWriter) WithHooks(hooks ...func()) ValueWriter {
	if len(hooks) == 0 {
		return writer
	}
	next := make([]func(), len(writer.Hooks), len(writer.Hooks)+len(hooks))
	copy(next, writer.Hooks)
	writer.Hooks = append(next, hooks...)
	return writer
}

func (context ValueContext) WithHook(hook func()) ValueContext {
	context.Writer = context.Writer.WithHooks(hook)
	return context
}
