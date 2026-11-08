package header

import (
	"io"
	"os"
)

// ReadHeader reads the first 16 bytes of a file.
func ReadHeader(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	header := make([]byte, 16)
	if _, err := io.ReadFull(f, header); err != nil {
		return nil, err
	}
	return header, nil
}
