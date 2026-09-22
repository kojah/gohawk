package borrowedstorage

import "bytes"

var shared bytes.Buffer
var replacement *bytes.Buffer

type cache struct {
	body *bytes.Buffer
	copy *bytes.Buffer
}

func returnParameterView(source *bytes.Buffer) *bytes.Buffer {
	return bytes.NewBuffer(source.Bytes()) // want "bytes.NewBuffer takes ownership of storage still owned by the source bytes.Buffer; copy the bytes first"
}

func returnFieldView(owner *cache) *bytes.Buffer {
	return bytes.NewBuffer(owner.body.Next(1)) // want "bytes.NewBuffer takes ownership of storage still owned by the source bytes.Buffer; copy the bytes first"
}

func returnGlobalView() *bytes.Buffer {
	view := shared.AvailableBuffer()
	return bytes.NewBuffer(view) // want "bytes.NewBuffer takes ownership of storage still owned by the source bytes.Buffer; copy the bytes first"
}

func storeInExternalField(owner *cache) {
	owner.copy = bytes.NewBuffer(owner.body.Bytes()) // want "bytes.NewBuffer takes ownership of storage still owned by the source bytes.Buffer; copy the bytes first"
}

func storeInGlobal(source *bytes.Buffer) {
	replacement = bytes.NewBuffer(source.Bytes()) // want "bytes.NewBuffer takes ownership of storage still owned by the source bytes.Buffer; copy the bytes first"
}

func returnBothLocalOwners() (*bytes.Buffer, *bytes.Buffer) {
	var source bytes.Buffer
	source.WriteString("payload")
	return &source, bytes.NewBuffer(source.Bytes()) // want "bytes.NewBuffer takes ownership of storage still owned by the source bytes.Buffer; copy the bytes first"
}

func transferLocalStorage() *bytes.Buffer {
	var source bytes.Buffer
	source.WriteString("payload")
	return bytes.NewBuffer(source.Bytes())
}

func keepNewOwnerLocal(source *bytes.Buffer) int {
	copy := bytes.NewBuffer(source.Bytes())
	return copy.Len()
}

func returnExplicitClone(source *bytes.Buffer) *bytes.Buffer {
	return bytes.NewBuffer(bytes.Clone(source.Bytes()))
}

func returnAppendCopy(source *bytes.Buffer) *bytes.Buffer {
	return bytes.NewBuffer(append([]byte(nil), source.Bytes()...))
}

func returnReaderView(source *bytes.Buffer) *bytes.Reader {
	return bytes.NewReader(source.Bytes())
}

func explicitSliceTransfer(source []byte) *bytes.Buffer {
	return bytes.NewBuffer(source)
}

func localStructDoesNotEscape(source *bytes.Buffer) int {
	owner := struct{ buffer *bytes.Buffer }{buffer: bytes.NewBuffer(source.Bytes())}
	return owner.buffer.Len()
}
