// Package wrapping holds constructors whose summaries the importing package
// depends on. A constructor that delegates the store of its argument to an
// unexported helper is summarized here, where that helper's body is visible;
// the importer sees only the summary, so the fact has to carry the answer.
package wrapping

import "io"

// Reader keeps a source and releases nothing, so wrapping a file in one leaves
// the caller holding the obligation to close it.
type Reader struct{ source io.Reader }

func (r *Reader) reset(source io.Reader) { r.source = source }

func (r *Reader) Read(p []byte) (int, error) { return r.source.Read(p) }

// NewReader delegates the store, as bufio.NewReader does through
// (*bufio.Reader).reset. The parameter reaches the returned value only inside
// reset, so a summary that stops at this body concludes the callee kept the
// source for itself.
func NewReader(source io.Reader) *Reader {
	wrapped := new(Reader)
	wrapped.reset(source)
	return wrapped
}

// NewReaderFastPath returns the argument itself when it is already a Reader,
// the shape bufio.NewReaderSize takes.
func NewReaderFastPath(source io.Reader) *Reader {
	if already, ok := source.(*Reader); ok {
		return already
	}
	wrapped := new(Reader)
	wrapped.reset(source)
	return wrapped
}

// Closer releases the source it wraps, so handing it a file does transfer the
// obligation to close that file.
type Closer struct{ source io.Closer }

func (c *Closer) reset(source io.Closer) { c.source = source }

func (c *Closer) Close() error { return c.source.Close() }

func NewCloser(source io.Closer) *Closer {
	wrapped := new(Closer)
	wrapped.reset(source)
	return wrapped
}


// DirectCloser stores its argument directly and closes it, so every step is
// provable: the argument can be closed, the field holds it, and Close closes
// that field. The field is an interface, which is how a wrapper holds what it
// was given, and a summary that cannot read a cleanup method there says the
// caller still owns the argument.
type DirectCloser struct{ inner io.Closer }

func NewDirectCloser(source io.Closer) *DirectCloser { return &DirectCloser{inner: source} }

func (d *DirectCloser) Close() error { return d.inner.Close() }

// SplitOwner keeps what it was given in borrowed and something of its own in
// owned, closing only owned. The store into borrowed is delegated, so reading
// the constructor alone finds no field and cannot say whether anything
// releases what the caller handed over.
type SplitOwner struct {
	borrowed io.Closer
	owned    io.Closer
}

func (s *SplitOwner) reset(source io.Closer) { s.borrowed = source }

func (s *SplitOwner) Close() error { return s.owned.Close() }

func NewSplitOwner(source io.Closer) *SplitOwner {
	split := new(SplitOwner)
	split.reset(source)
	return split
}
