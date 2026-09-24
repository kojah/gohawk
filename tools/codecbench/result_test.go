package codecbench

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/kojah/gohawk/internal/factcodec"
	"github.com/kojah/gohawk/internal/passes/resultfacts"
	"github.com/kojah/gohawk/tools/codecbench/resultpb"
	"google.golang.org/protobuf/proto"
)

func resultFixture(count int) resultfacts.Fact {
	fact := resultfacts.Fact{Version: 3}
	for index := range count {
		fact.Results = append(fact.Results, resultfacts.AlwaysNonNil)
		fact.Relations = append(fact.Relations, resultfacts.Relation{
			Result: index, Kind: resultfacts.NonNilWhenResultNil, Operand: (index + 1) % count,
		})
	}
	return fact
}

// Include conversion allocations rather than timing only prebuilt messages.
func encodeProto(fact resultfacts.Fact) ([]byte, error) {
	wire := &resultpb.ResultFact{Version: int32(fact.Version), NeverReturns: fact.NeverReturns}
	wire.Results = make([]uint32, len(fact.Results))
	for index, result := range fact.Results {
		wire.Results[index] = uint32(result)
	}
	wire.Relations = make([]*resultpb.Relation, len(fact.Relations))
	for index, relation := range fact.Relations {
		wire.Relations[index] = &resultpb.Relation{Kind: uint32(relation.Kind), Result: int32(relation.Result), Operand: int32(relation.Operand)}
	}
	return (proto.MarshalOptions{Deterministic: true}).Marshal(wire)
}

func decodeProto(data []byte) (resultfacts.Fact, error) {
	var wire resultpb.ResultFact
	if err := (proto.UnmarshalOptions{RecursionLimit: 64}).Unmarshal(data, &wire); err != nil {
		return resultfacts.Fact{}, err
	}
	fact := resultfacts.Fact{Version: int(wire.Version), NeverReturns: wire.NeverReturns}
	if wire.Results != nil {
		fact.Results = make([]resultfacts.Guarantee, len(wire.Results))
	}
	for index, result := range wire.Results {
		if result > uint32(resultfacts.AlwaysFalse) {
			return resultfacts.Fact{}, fmt.Errorf("invalid guarantee %d", result)
		}
		fact.Results[index] = resultfacts.Guarantee(result)
	}
	if wire.Relations != nil {
		fact.Relations = make([]resultfacts.Relation, len(wire.Relations))
	}
	for index, relation := range wire.Relations {
		if relation.Kind > uint32(resultfacts.ReturnsParameter) {
			return resultfacts.Fact{}, fmt.Errorf("invalid relation %d", relation.Kind)
		}
		fact.Relations[index] = resultfacts.Relation{Kind: resultfacts.RelationKind(relation.Kind), Result: int(relation.Result), Operand: int(relation.Operand)}
	}
	return fact, nil
}

var codecs = []struct {
	name   string
	encode func(resultfacts.Fact) ([]byte, error)
	decode func([]byte) (resultfacts.Fact, error)
}{
	{"json", func(f resultfacts.Fact) ([]byte, error) { return json.Marshal(&f) }, func(b []byte) (resultfacts.Fact, error) {
		var f resultfacts.Fact
		err := json.Unmarshal(b, &f)
		return f, err
	}},
	{"cbor", func(f resultfacts.Fact) ([]byte, error) { return factcodec.Encode(&f) }, func(b []byte) (resultfacts.Fact, error) {
		var f resultfacts.Fact
		err := factcodec.Decode(b, &f)
		return f, err
	}},
	{"protobuf", encodeProto, decodeProto},
}

func TestResultCodecs(t *testing.T) {
	for _, count := range []int{0, 2, 16} {
		want := resultFixture(count)
		for _, codec := range codecs {
			data, err := codec.encode(want)
			if err != nil {
				t.Fatal(err)
			}
			got, err := codec.decode(data)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("%s/%d: got %+v, error %v, want %+v", codec.name, count, got, err, want)
			}
		}
	}
}

func BenchmarkResultCodec(b *testing.B) {
	for _, count := range []int{0, 2, 16} {
		value := resultFixture(count)
		for _, codec := range codecs {
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
