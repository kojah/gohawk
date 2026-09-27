package resourcelifetime

import (
	"errors"
	"io"
	"os"
)

// A helper that closes every element of the slice it is handed, on every
// normal return, settles a local collection passed to it whole. Anything less
// than every element on every return, or a slice the helper keeps, leaves the
// collection unknown.

// closeEachFile closes every element on every return.
func closeEachFile(files []*os.File) {
	for _, file := range files {
		_ = file.Close()
	}
}

// closeFirstFile closes only the first element.
func closeFirstFile(files []*os.File) {
	if len(files) > 0 {
		_ = files[0].Close()
	}
}

// closeUntilError stops at the first failure.
func closeUntilError(files []*os.File) error {
	for _, file := range files {
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

// closeSkippingNil skips some elements.
func closeSkippingNil(files []*os.File, skip func(*os.File) bool) {
	for _, file := range files {
		if skip(file) {
			continue
		}
		_ = file.Close()
	}
}

var keptFiles [][]*os.File

// closeAndKeep closes every element but also keeps the slice.
func closeAndKeep(files []*os.File) {
	keptFiles = append(keptFiles, files)
	for _, file := range files {
		_ = file.Close()
	}
}

// closeOther closes a different slice.
func closeOther(files, others []*os.File) {
	_ = files
	for _, file := range others {
		_ = file.Close()
	}
}

// closeEachUnlessEmpty returns early without closing when asked.
func closeEachUnlessEarly(files []*os.File, early bool) {
	if early {
		return
	}
	for _, file := range files {
		_ = file.Close()
	}
}

// closeEachViaCallback releases through a callback it may not call.
func closeEachViaCallback(files []*os.File, release func(io.Closer)) {
	for _, file := range files {
		if release != nil {
			release(file)
		}
	}
}

// Accepted: the helper closes every element.
func collectionClosedByHelper(paths []string) error {
	var files []*os.File
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		files = append(files, file)
	}
	closeEachFile(files)
	return nil
}

// Accepted: helpers that close only some elements leave the collection
// unknown rather than reported.
func collectionPartlyClosedByHelpers(paths []string, skip func(*os.File) bool, early bool, release func(io.Closer)) {
	var first, until, skipping, other, unless, callback []*os.File
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		switch len(path) % 6 {
		case 0:
			first = append(first, file)
		case 1:
			until = append(until, file)
		case 2:
			skipping = append(skipping, file)
		case 3:
			other = append(other, file)
		case 4:
			unless = append(unless, file)
		default:
			callback = append(callback, file)
		}
	}
	closeFirstFile(first)
	_ = closeUntilError(until)
	closeSkippingNil(skipping, skip)
	closeOther(nil, other)
	closeEachUnlessEarly(unless, early)
	closeEachViaCallback(callback, release)
}

// Accepted: the helper keeps the slice.
func collectionKeptByHelper(paths []string) {
	var files []*os.File
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			files = append(files, file)
		}
	}
	closeAndKeep(files)
}

// Accepted: the caller hands over a sub-slice or a copy, not the collection.
func collectionSubsliceToHelper(paths []string) {
	var files []*os.File
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			files = append(files, file)
		}
	}
	closeEachFile(files[1:])
	copied := make([]*os.File, len(files))
	copy(copied, files)
	closeEachFile(copied)
}

// Accepted: a helper reached through a function value has no summary.
func collectionClosedByUnknownHelper(paths []string, closeAll func([]*os.File)) {
	var files []*os.File
	for _, path := range paths {
		if file, err := os.Open(path); err == nil {
			files = append(files, file)
		}
	}
	closeAll(files)
}

// The early error return skips the helper that would close every file.
func collectionClosedOnlyOnSuccess(paths []string, check func() error) error {
	var files []*os.File
	for _, path := range paths {
		file, err := os.Open(path) // want "owned resource from os.Open is not released"
		if err != nil {
			continue
		}
		files = append(files, file)
	}
	if err := check(); err != nil {
		return errors.New("check failed")
	}
	closeEachFile(files)
	return nil
}
