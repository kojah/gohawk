package factcodec_test

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kojah/gohawk/internal/factcodec"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/passes/resultfacts"
)

type lifecycleEnvelope struct {
	factcodec.Envelope[lifecyclefacts.Fact]
}

type lifecyclePayload = factcodec.Envelope[lifecyclefacts.Fact]

type privateLifecycleEnvelope struct {
	lifecyclePayload
}

// Populate all exported fields so newly added wire fields join this regression
// automatically. The values exercise representation, not domain proof validity.
func TestEverySummaryFieldRoundTrips(t *testing.T) {
	for _, value := range []any{
		new(lifecyclefacts.Fact), new(lifecyclefacts.CleanupFact), new(lifecyclefacts.SummarizedPackage),
		new(concurrencyfacts.Fact), new(resultfacts.Fact),
	} {
		t.Run(reflect.TypeOf(value).String(), func(t *testing.T) {
			populateWireFields(t, reflect.ValueOf(value).Elem(), 0)
			data, err := factcodec.Encode(value)
			if err != nil {
				t.Fatal(err)
			}
			decoded := reflect.New(reflect.TypeOf(value).Elem()).Interface()
			if err := factcodec.Decode(data, decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(value, decoded) {
				t.Fatal("serialization changed a summary field")
			}
		})
	}
}

func populateWireFields(t *testing.T, value reflect.Value, depth int) {
	t.Helper()
	if depth > 16 {
		t.Fatal("unexpected recursive publication model")
	}
	switch value.Kind() {
	case reflect.Struct:
		for info, field := range value.Fields() {
			// Unexported fields are not on the wire.
			if info.IsExported() {
				populateWireFields(t, field, depth+1)
			}
		}
	case reflect.Pointer:
		value.Set(reflect.New(value.Type().Elem()))
		populateWireFields(t, value.Elem(), depth+1)
	case reflect.Slice:
		value.Set(reflect.MakeSlice(value.Type(), 2, 2))
		for index := range value.Len() {
			populateWireFields(t, value.Index(index), depth+1)
		}
	case reflect.Array:
		for index := range value.Len() {
			populateWireFields(t, value.Index(index), depth+1)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value.SetInt(-17)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		value.SetUint(129)
	case reflect.Bool:
		value.SetBool(true)
	case reflect.String:
		value.SetString("field:1/λ\x00value")
	default:
		t.Fatalf("add coverage for wire kind %v", value.Type())
	}
}

// Keep the pre-optimization representation only as a benchmark baseline.
type exposedJSONFact lifecyclefacts.Fact

func (fact *exposedJSONFact) GobEncode() ([]byte, error) {
	return json.Marshal(fact) //nolint:musttag // Benchmark the exact historical default-field encoding.
}

func (fact *exposedJSONFact) GobDecode(data []byte) error {
	return json.Unmarshal(data, fact) //nolint:musttag // Benchmark the exact historical default-field decoding.
}

func benchmarkSummary() lifecyclefacts.Fact {
	return lifecyclefacts.Fact{
		Discharges: []lifecyclefacts.Discharge{{Parameter: 0, Method: "Close"}, {Parameter: 0, Method: "Close", Path: "field:1"}},
		Heap: &heapmodel.HeapSummary{
			Version: heapmodel.SummaryVersion,
			Edges: []heapmodel.HeapEdge{{
				From: heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapResult}},
				To:   heapmodel.HeapTarget{Kind: heapmodel.HeapTargetFresh, Origin: "factory:17"}, Must: true,
			}},
		},
	}
}

// Match checker's fresh-stream, two-encode/one-decode determinism validation.
// The sparse summary uses real domain types; it is not a workload distribution.
func BenchmarkFactRoundTrip(b *testing.B) {
	value := benchmarkSummary()
	for _, test := range []struct {
		name  string
		value any
		fresh func() any
	}{
		{"exposed-json", (*exposedJSONFact)(&value), func() any { return new(exposedJSONFact) }},
		{"opaque", &lifecycleEnvelope{factcodec.Wrap(value)}, func() any { return new(lifecycleEnvelope) }},
		{"private-envelope", &privateLifecycleEnvelope{factcodec.Wrap(value)}, func() any { return new(privateLifecycleEnvelope) }},
	} {
		b.Run(test.name, func(b *testing.B) {
			var size bytes.Buffer
			if err := gob.NewEncoder(&size).Encode(test.value); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				var first, second bytes.Buffer
				if err := gob.NewEncoder(&first).Encode(test.value); err != nil {
					b.Fatal(err)
				}
				if err := gob.NewEncoder(&second).Encode(test.value); err != nil {
					b.Fatal(err)
				}
				if !bytes.Equal(first.Bytes(), second.Bytes()) {
					b.Fatal("nondeterministic fact")
				}
				if err := gob.NewDecoder(&first).Decode(test.fresh()); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(size.Len()), "wire-B")
		})
	}
}

func BenchmarkPayload(b *testing.B) {
	value := benchmarkSummary()
	for _, codec := range []struct {
		name   string
		encode func(any) ([]byte, error)
		decode func([]byte, any) error
	}{
		{"json", json.Marshal, json.Unmarshal},
		{"cbor", factcodec.Encode, factcodec.Decode},
	} {
		b.Run(codec.name, func(b *testing.B) {
			data, err := codec.encode(&value)
			if err != nil {
				b.Fatal(err)
			}
			b.Run("encode", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := codec.encode(&value); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(len(data)), "wire-B")
			})
			b.Run("decode", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					var decoded lifecyclefacts.Fact
					if err := codec.decode(data, &decoded); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	want := benchmarkSummary()
	var data bytes.Buffer
	if err := gob.NewEncoder(&data).Encode(&privateLifecycleEnvelope{factcodec.Wrap(want)}); err != nil {
		t.Fatal(err)
	}
	var got privateLifecycleEnvelope
	if err := gob.NewDecoder(&data).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Value(), want) {
		t.Fatalf("got %#v, want %#v", got.Value(), want)
	}
	want.Heap.Edges[0].To.Origin = "mutated"
	if got.Value().Heap.Edges[0].To.Origin == "mutated" {
		t.Fatal("decoded fact aliases input")
	}
}
