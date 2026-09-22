package borrowedstorage

import "bytes"

//gohawk:example flagged
func snapshot(source *bytes.Buffer) *bytes.Buffer {
	return bytes.NewBuffer(source.Bytes()) // want "bytes.NewBuffer takes ownership of storage still owned by the source bytes.Buffer; copy the bytes first"
}

//gohawk:example end

//gohawk:example ok
func snapshotSafely(source *bytes.Buffer) *bytes.Buffer {
	return bytes.NewBuffer(bytes.Clone(source.Bytes()))
}

//gohawk:example end
