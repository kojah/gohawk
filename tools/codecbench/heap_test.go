package codecbench

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/kojah/gohawk/internal/factcodec"
	"github.com/kojah/gohawk/internal/heapmodel"
)

func heapFixture(count int) heapmodel.HeapSummary {
	value := heapmodel.HeapSummary{Version: heapmodel.SummaryVersion}
	for index := range count {
		slot := heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter, Index: index % 4}, Path: fmt.Sprintf("field:%d", index)}
		value.Edges = append(value.Edges, heapmodel.HeapEdge{From: slot, To: heapmodel.HeapTarget{Kind: heapmodel.HeapTargetFresh, Object: index, Origin: "example.Factory:42"}, Must: true})
		value.Effects = append(value.Effects, heapmodel.HeapEffect{Slot: slot, Escape: heapmodel.HeapEscapedField, Every: true})
		value.Requires = append(value.Requires, heapmodel.HeapRequirement{Slot: slot, Kind: heapmodel.HeapRequiresMethod, Method: "Read"})
	}
	return value
}

var heapCodecs = []struct {
	name   string
	encode func(heapmodel.HeapSummary) ([]byte, error)
	decode func([]byte) (heapmodel.HeapSummary, error)
}{
	{"json", func(f heapmodel.HeapSummary) ([]byte, error) { return json.Marshal(&f) }, func(b []byte) (heapmodel.HeapSummary, error) {
		var f heapmodel.HeapSummary
		err := json.Unmarshal(b, &f)
		return f, err
	}},
	{"cbor", func(f heapmodel.HeapSummary) ([]byte, error) { return factcodec.Encode(&f) }, func(b []byte) (heapmodel.HeapSummary, error) {
		var f heapmodel.HeapSummary
		err := factcodec.Decode(b, &f)
		return f, err
	}},
	{"protobuf", encodeHeapProto, decodeHeapProto},
}

func TestHeapCodecs(t *testing.T) {
	for _, count := range []int{0, 2, 32} {
		want := heapFixture(count)
		for _, codec := range heapCodecs {
			data, err := codec.encode(want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := codec.decode(data)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("%s/%d: round-trip mismatch: %v", codec.name, count, err)
			}
		}
	}
}

func BenchmarkHeapCodec(b *testing.B) {
	for _, count := range []int{2, 32} {
		value := heapFixture(count)
		for _, codec := range heapCodecs {
			b.Run(fmt.Sprintf("%d/%s", count, codec.name), func(b *testing.B) {
				data, err := codec.encode(value)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				for b.Loop() {
					encoded, err := codec.encode(value)
					if err != nil {
						b.Fatal(err)
					}
					if _, err := codec.decode(encoded); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(len(data)), "wire-B")
			})
		}
	}
}
