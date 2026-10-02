// Private entry chains permit reclaim-only resources, while repeated calls,
// escaped helpers and effectful cleanup retain their obligations.
package main

import (
	"compress/gzip"
	"database/sql"
	"errors"
	"os"
)

var database *sql.DB
var callback = escaped

func main() {
	_ = bridge(len(os.Args) > 1)
	repeated()
	repeated()
	for range os.Args {
		looped()
	}
	escaped()
	flush()
	transaction()
}

func bridge(fail bool) error { return once(fail) }

func once(fail bool) error {
	file, err := os.Create("patterns")
	if err != nil {
		return err
	}
	if fail {
		return errors.New("failed")
	}
	return file.Close()
}

func repeated() {
	file, err := os.Open("config") // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	_ = file.Name()
}

func looped() {
	file, err := os.Open("config") // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	_ = file.Name()
}

func escaped() {
	file, err := os.Open("config") // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	_ = file.Name()
}

func flush() {
	file, err := os.Create("out.gz")
	if err != nil {
		return
	}
	defer file.Close()
	compressor := gzip.NewWriter(file) // want "owned resource from gzip.NewWriter is not released"
	_, _ = compressor.Write([]byte("data"))
}

func transaction() {
	tx, err := database.Begin() // want "owned resource from sql.Begin is not released"
	if err != nil {
		return
	}
	_, _ = tx.Exec("insert into t values (1)")
}
