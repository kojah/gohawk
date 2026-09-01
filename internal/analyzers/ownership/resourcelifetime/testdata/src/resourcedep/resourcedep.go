package resourcedep

import "os"

func Close(file *os.File) error { return file.Close() }

func MaybeClose(file *os.File, enabled bool) error {
	if enabled {
		return file.Close()
	}
	return nil
}
