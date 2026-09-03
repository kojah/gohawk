package resourcelifetime

import "os"

// This file covers a resource that reaches an in-package callee only nested
// inside an aggregate argument. The callee transfers the resource to the value
// it returns, but through more than one return path, so the transfer cannot be
// proven. A parameter-level completion proof follows the parameter value, not a
// resource buried in one of its fields, so the resource is unknown at that
// boundary rather than a reported leak. A resource handed to such a callee
// directly stays visible to the proof, so a genuine leak there is still
// reported. oss-rebuild wraps a zip reader this way before handing it to a
// loader whose ownership cannot be proven:
// https://github.com/google/oss-rebuild/blob/9ce0528dd68bf209b52cc9fdc90bd63742cbb3a0/pkg/sysgraph/sgstorage/loader.go#L173-L179

// holder owns a file and releases it on Close.
type holder struct{ file *os.File }

func (h *holder) Close() error { return h.file.Close() }

// wrapAndLoad transfers the held file to the returned holder, but its two
// return paths, one of them through relayHolder, leave the transfer unprovable.
func wrapAndLoad(h *holder, multi bool) (*holder, error) {
	if multi {
		return relayHolder(h)
	}
	return h, nil
}

func relayHolder(h *holder) (*holder, error) { return h, nil }

// transferredThroughAggregate wraps the file in a holder and hands it to
// wrapAndLoad, which transfers it. The unprovable transfer is unknown, so no
// leak is reported.
func transferredThroughAggregate(path string, multi bool) (*holder, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return wrapAndLoad(&holder{file: file}, multi)
}

// dropFile receives the file directly and neither releases nor returns it.
func dropFile(file *os.File, multi bool) error {
	if multi {
		return relayDrop(file)
	}
	return nil
}

func relayDrop(file *os.File) error { return nil }

// droppedThroughDirectCall passes the file directly, not nested in an
// aggregate, to a callee that drops it, so it leaks and is still reported.
func droppedThroughDirectCall(path string, multi bool) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	return dropFile(file, multi)
}

// request carries a file to a sender that reports only an error.
type request struct{ body *os.File }

// upload reports a status and hands nothing back that could own the file.
func upload(r *request) error { return nil }

// leakedThroughAggregateWithoutTransfer stores the file in a request handed to
// a call whose error alone is propagated, so nothing took ownership of the file
// and it leaks. buildkite/agent uploads an artifact body this way.
func leakedThroughAggregateWithoutTransfer(path string) error {
	file, err := os.Open(path) // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	return upload(&request{body: file})
}
