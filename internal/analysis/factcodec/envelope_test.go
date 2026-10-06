package factcodec

import (
	"bytes"
	"reflect"
	"sync"
	"testing"
)

type testPayload struct {
	Mask  uint64
	Items []int
	Names map[string]int
	Child *testPayload
}

func TestEnvelopeValues(t *testing.T) {
	for _, value := range []testPayload{
		{},
		{Mask: ^uint64(0), Items: []int{}, Names: map[string]int{}, Child: &testPayload{}},
		{Items: []int{-1, 0, 42}, Names: map[string]int{"z": 1, "a": 2}},
	} {
		envelope := Wrap(value)
		data, err := envelope.GobEncode()
		if err != nil {
			t.Fatal(err)
		}
		var decoded Envelope[testPayload]
		if err := decoded.GobDecode(data); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(value, decoded.Value()) {
			t.Fatalf("got %#v, want %#v", decoded.Value(), value)
		}
		fresh, err := Encode(&value)
		if err != nil || !bytes.Equal(data, fresh) {
			t.Fatalf("unstable encoding: %v", err)
		}
	}
}

func TestEnvelopeReplacement(t *testing.T) {
	envelope := Wrap(testPayload{Mask: 99, Items: []int{1}, Child: &testPayload{Mask: 2}})
	if _, err := envelope.GobEncode(); err != nil {
		t.Fatal(err)
	}
	data, err := Encode(&testPayload{})
	if err != nil {
		t.Fatal(err)
	}
	if err := envelope.GobDecode(data); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(envelope.Value(), testPayload{}) {
		t.Fatal("decode retained old fields")
	}
	encoded, err := envelope.GobEncode()
	if err != nil || !bytes.Equal(encoded, data) {
		t.Fatal("decode retained old encoding")
	}
}

func TestEnvelopeRejectsInvalidData(t *testing.T) {
	valid, err := Encode(testPayload{Mask: 1})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"legacy-json":   []byte(`{"Mask":1}`),
		"wrong-version": []byte("GHF\x02\xa0"),
		"truncated":     valid[:len(valid)-1],
		"trailing":      append(bytes.Clone(valid), 0),
		"null":          []byte(wireHeader + "\xf6"),
		"undefined":     []byte(wireHeader + "\xf7"),
		"duplicate":     []byte(wireHeader + "\xa2\x64Mask\x01\x64Mask\x02"),
		"unknown-field": []byte(wireHeader + "\xa1\x61X\x01"),
		"overflow":      []byte(wireHeader + "\xa1\x64Mask\x20"),
		"huge-array":    []byte(wireHeader + "\x9a\xff\xff\xff\xff"),
		"oversized":     make([]byte, maxPayloadBytes+len(wireHeader)+1),
	} {
		t.Run(name, func(t *testing.T) {
			envelope := Wrap(testPayload{Mask: 42})
			if err := envelope.GobDecode(data); err == nil {
				t.Fatal("accepted invalid fact")
			}
			if envelope.Value().Mask != 42 {
				t.Fatal("failed decode changed evidence")
			}
		})
	}
}

func TestEnvelopeConcurrentCopies(t *testing.T) {
	envelope := Wrap(testPayload{Names: map[string]int{"z": 3, "a": 7}})
	want, err := Encode(envelope.Value())
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 16 {
		group.Go(func() {
			copy := envelope
			for range 20 {
				got, err := copy.GobEncode()
				if err != nil || !bytes.Equal(got, want) {
					t.Errorf("concurrent encoding: %x, %v", got, err)
				}
			}
		})
	}
	group.Wait()
}

func FuzzEnvelopeDecode(f *testing.F) {
	data, err := Encode(&testPayload{Items: []int{1, 2}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte(wireHeader + "\xf6"))
	f.Fuzz(func(t *testing.T, data []byte) {
		envelope := Wrap(testPayload{Mask: 99})
		if err := envelope.GobDecode(data); err != nil {
			return
		}
		encoded, err := envelope.GobEncode()
		if err != nil {
			t.Fatal(err)
		}
		var again Envelope[testPayload]
		if err := again.GobDecode(encoded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(envelope.Value(), again.Value()) {
			t.Fatal("unstable decode")
		}
	})
}
