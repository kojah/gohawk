package deferinloop

import (
	"archive/zip"
	"strings"
)

// A string or number cannot hold a reference, so passing one read from the
// resource's owner to a callee cannot hand the resource over. len and cap only
// read a length. Neither settles or hides the per-iteration obligation.
// Real-world form: ForceCLI's unpackResources defers each zip entry's Close
// inside the loop over the archive's files,
// https://github.com/ForceCLI/force/blob/662af739b980a568fa55e3a4d7efe65cf2ec15b1/command/fetch.go#L321-L330
func deferredEntryCloses(archives []string) error {
	for _, name := range archives {
		archive, err := zip.OpenReader(name)
		if err != nil {
			return err
		}
		defer archive.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
		if len(archive.File) == 0 {
			continue
		}
		for _, entry := range archive.File {
			reader, err := entry.Open()
			if err != nil {
				return err
			}
			defer reader.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
			if strings.HasPrefix(entry.Name, "__") {
				continue
			}
		}
	}
	return nil
}

// Ranging over the archive's files takes their length, which neither
// releases nor retains the archive.
func deferredArchivePerResource(resources map[string]string) error {
	for _, name := range resources {
		archive, err := zip.OpenReader(name)
		if err != nil {
			return err
		}
		defer archive.Close() // want "deferred cleanup runs after the loop instead of after this iteration"
		for _, entry := range archive.File {
			println(entry.Name)
		}
	}
	return nil
}
