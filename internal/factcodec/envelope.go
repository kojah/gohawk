package factcodec

import (
	"fmt"
	"sync"
)

// Envelope hides a summary's fields from gob's type-descriptor traversal.
// Embed it in a domain-owned fact type; the domain still owns publication and
// validation. Values and everything reachable from them must stay immutable
// after publication. Each decode produces an independently owned value.
type Envelope[T any] struct {
	value    T
	encoding *cachedEncoding
}

// This indirection matters: go/analysis copies fact values with reflection.
// Copies share the immutable payload and its cache, never copy a used mutex.
type cachedEncoding struct {
	once sync.Once
	data []byte
	err  error
}

// Wrap prepares a summary for publication without copying its reachable data.
func Wrap[T any](value T) Envelope[T] { return Envelope[T]{value: value, encoding: &cachedEncoding{}} }

// Value returns the summary. Its slices, maps, and pointers remain read-only.
func (envelope *Envelope[T]) Value() T { return envelope.value }

// AFact marks the enclosing domain type as an analysis fact.
func (*Envelope[T]) AFact() {}

// String preserves the domain's human-readable fact rendering.
func (envelope *Envelope[T]) String() string { return fmt.Sprint(&envelope.value) }

// GobEncode encodes only the payload, not its nested gob descriptors. The
// returned bytes belong to the envelope and must not be modified by callers.
func (envelope *Envelope[T]) GobEncode() ([]byte, error) {
	if envelope.encoding == nil {
		return Encode(&envelope.value)
	}
	cache := envelope.encoding
	cache.once.Do(func() { cache.data, cache.err = Encode(&envelope.value) })
	return cache.data, cache.err
}

// GobDecode replaces the payload only after a successful decode. Decode into a
// fresh value so reused receivers cannot retain missing fields or share storage.
func (envelope *Envelope[T]) GobDecode(data []byte) error {
	var value T
	if err := Decode(data, &value); err != nil {
		return err
	}
	*envelope = Wrap(value)
	return nil
}
