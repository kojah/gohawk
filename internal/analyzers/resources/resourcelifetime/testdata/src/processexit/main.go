// Package main covers resources that program exit reclaims. An acquisition
// that runs at most once in main.main cannot accumulate, and every way out of
// main ends the process, so a descriptor or connection it leaves open is
// closed by the operating system. Cleanups with an effect that exit would
// lose, and acquisitions that can repeat, are still reported.
package main

import (
	"compress/gzip"
	"database/sql"
	"net/http"
	"os"
)

var db *sql.DB

func main() {
	logFile, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		os.Exit(1)
	}
	_, _ = logFile.WriteString("started\n")

	response, err := http.Get("http://example.com/health")
	if err != nil {
		return
	}
	_ = response.StatusCode

	rows, err := db.Query("select 1")
	if err != nil {
		return
	}
	_ = rows.Next()

	// Serving in a loop acquires once per iteration, so the files accumulate.
	for _, name := range os.Args[1:] {
		file, err := os.Open(name) // want "owned resource from os.Open is not released"
		if err != nil {
			continue
		}
		_ = file.Name()
	}

	// A closure can run more than once, even when declared in main.
	open := func(name string) {
		file, err := os.Open(name) // want "owned resource from os.Open is not released"
		if err != nil {
			return
		}
		_ = file.Name()
	}
	open("a")

	// Closing a compressor flushes its buffered data; exit would lose it.
	output, err := os.Create("out.gz")
	if err != nil {
		return
	}
	defer output.Close()
	compressor := gzip.NewWriter(output) // want "owned resource from gzip.NewWriter is not released"
	_, _ = compressor.Write([]byte("data"))

	// A transaction left open at exit rolls back instead of committing.
	transaction, err := db.Begin() // want "owned resource from sql.Begin is not released"
	if err != nil {
		return
	}
	_, _ = transaction.Exec("insert into t values (1)")
}
