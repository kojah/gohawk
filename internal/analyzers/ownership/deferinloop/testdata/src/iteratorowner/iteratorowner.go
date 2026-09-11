package iteratorowner

import "os"

type Rows struct {
	file      *os.File
	remaining int
}

func Open(name string) (*Rows, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	return &Rows{file: file, remaining: 1}, nil
}

func (rows *Rows) Close() error { return rows.file.Close() }

func (rows *Rows) Next() bool {
	if rows.remaining == 0 {
		return false
	}
	rows.remaining--
	return true
}
