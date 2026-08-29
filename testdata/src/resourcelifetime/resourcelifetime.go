package resourcelifetime

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"time"
)

func leakedFile() error {
	file, err := os.CreateTemp("", "leak") // want "owned resource from os.CreateTemp is not released on every return path"
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func closedFile() error {
	file, err := os.CreateTemp("", "closed")
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

func leakedTimer() {
	timer := time.NewTimer(time.Second) // want "owned resource from time.NewTimer is not released on every return path"
	_ = timer
}

func stoppedTimer() {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
}

func transferredFile() (*os.File, error) {
	return os.CreateTemp("", "transfer")
}

type owner struct {
	timer *time.Timer
	file  *os.File
}

func transferredTimer(target *owner) {
	target.timer = time.NewTimer(time.Second)
}

func partiallyConstructedOwner(fail bool) (*owner, error) {
	file, err := os.Open("state") // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return nil, err
	}
	result := &owner{file: file}
	if fail {
		return nil, errors.New("finish initialization")
	}
	return result, nil
}

func cleanedPartialOwner(fail bool) (*owner, error) {
	file, err := os.Open("state")
	if err != nil {
		return nil, err
	}
	result := &owner{file: file}
	if fail {
		_ = file.Close()
		return nil, errors.New("finish initialization")
	}
	return result, nil
}

func returnedOwner() (*owner, error) {
	file, err := os.Open("state")
	if err != nil {
		return nil, err
	}
	return &owner{file: file}, nil
}

func timerCommand() func() {
	timer := time.NewTimer(time.Second)
	return func() {
		<-timer.C
		timer.Stop()
	}
}

func leakedResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	_ = response
	return nil
}

func closedResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return nil
}

func leakedRows(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "SELECT 1") // want "owned resource from sql.QueryContext is not released on every return path"
	if err != nil {
		return err
	}
	_ = rows
	return nil
}

func closedRows(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, "SELECT 1")
	if err != nil {
		return err
	}
	defer rows.Close()
	return nil
}

func leakedStatement(ctx context.Context, database *sql.DB) error {
	statement, err := database.PrepareContext(ctx, "SELECT 1") // want "owned resource from sql.PrepareContext is not released on every return path"
	if err != nil {
		return err
	}
	_ = statement
	return nil
}

func transactionOwnsStatement(ctx context.Context, transaction *sql.Tx) error {
	_, err := transaction.PrepareContext(ctx, "SELECT 1")
	return err
}

func leakedGzipWriter(destination io.Writer) {
	writer := gzip.NewWriter(destination) // want "owned resource from gzip.NewWriter is not released on every return path"
	_ = writer
}

func closedGzipReader() error {
	reader, err := gzip.NewReader(bytes.NewReader(nil))
	if err != nil {
		return err
	}
	defer reader.Close()
	return nil
}

func closedFileThroughDeferredParameter() error {
	file, err := os.CreateTemp("", "closed")
	if err != nil {
		return err
	}
	defer func(open *os.File) {
		_ = open.Close()
	}(file)
	return nil
}

func leakedZlibWriter(destination io.Writer) {
	writer := zlib.NewWriter(destination) // want "owned resource from zlib.NewWriter is not released on every return path"
	_ = writer
}
