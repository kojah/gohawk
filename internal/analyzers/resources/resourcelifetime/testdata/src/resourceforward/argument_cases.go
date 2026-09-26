package resourceforward

import (
	"os"
	"resourcedep"
)

// CloseFile fixes the imported helper's flag to the closing branch.
func CloseFile(file *os.File) error { return resourcedep.MaybeClose(file, true) }

// KeepFile fixes the same flag to the branch that leaves the file open.
func KeepFile(file *os.File) error { return resourcedep.MaybeClose(file, false) }
