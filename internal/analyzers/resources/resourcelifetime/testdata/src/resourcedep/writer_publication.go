package resourcedep

import "io"

type writerView struct{ writer io.Writer }

func (view *writerView) Write(data []byte) (int, error) { return view.writer.Write(data) }
func WrapWriter(writer io.Writer) io.Writer             { return &writerView{writer: writer} }

var publishedWriter io.Writer

func PublishWriter(writer io.Writer) { publishedWriter = writer }
func InspectWriter(writer io.Writer) { _, _ = writer.Write(nil) }
