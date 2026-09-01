package resourcelifetime

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"resourcedep"
	"testing"
	"time"
)

func importedHelperClosesFile() error {
	file, err := os.Open("fixture")
	if err != nil {
		return err
	}
	return resourcedep.Close(file)
}

func conditionalImportedHelperLeaksFile(enabled bool) error {
	file, err := os.Open("fixture") // want "owned resource from os.Open is not released on every return path"
	if err != nil {
		return err
	}
	return resourcedep.MaybeClose(file, enabled)
}

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

func closedFileThroughFailureHelper(fail bool) error {
	file, err := os.CreateTemp("", "closed-helper")
	if err != nil {
		return err
	}
	cleanup := func(err error) error {
		_ = file.Close()
		return err
	}
	if fail {
		return cleanup(errors.New("failed"))
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

type compositeReadCloser struct {
	io.Reader
	closers []func() error
}

func (c *compositeReadCloser) Close() error {
	for _, closeResource := range c.closers {
		_ = closeResource()
	}
	return nil
}

func transferredGzipReader(source io.Reader) (io.ReadCloser, error) {
	reader, err := gzip.NewReader(source)
	if err != nil {
		return nil, err
	}
	return &compositeReadCloser{Reader: reader, closers: []func() error{reader.Close}}, nil
}

func leakedWriterAsInterface() error {
	file, err := os.CreateTemp("", "writer") // want "owned resource from os.CreateTemp is not released on every return path"
	if err != nil {
		return err
	}
	var destination io.Writer = file
	_, err = destination.Write(nil)
	return err
}

func closedFileAfterIgnoredMissingPath(paths []string) error {
	for _, path := range paths {
		file, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return nil
}

func closedFileAfterIgnoredFSMissingPath(path string) error {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return file.Close()
}

func closedFileAfterLegacyMissingPath(path string) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return file.Close()
}

func closedFileAfterExistingPath(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return file.Close()
}

func leakedFileAfterExistingPath(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // want "owned resource from os.OpenFile is not released on every return path"
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_ = file
	return nil
}

func leakedFileOnNegatedExistingCheck(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // want "owned resource from os.OpenFile is not released on every return path"
	if !errors.Is(err, fs.ErrExist) {
		_ = file
		return nil
	}
	return err
}

func settleFileParameter(file *os.File) {
	_ = file.Close()
}

func fileClosedByHelper() error {
	file, err := os.Open("state")
	if err != nil {
		return err
	}
	settleFileParameter(file)
	return nil
}

type fileSource interface {
	Close() error
}

type realFileSource struct {
	file *os.File
}

func (source *realFileSource) Close() error {
	return source.file.Close()
}

func settleSource(source fileSource) {
	defer source.Close()
}

func aggregateClosedByHelper() error {
	file, err := os.Open("state")
	if err != nil {
		return err
	}
	settleSource(&realFileSource{file: file})
	return nil
}

type closerOwner struct {
	closers []io.Closer
}

type outerCloserOwner struct {
	inner *closerOwner
}

func transferredNestedOwner(target *outerCloserOwner) error {
	file, err := os.Open("state")
	if err != nil {
		return err
	}
	target.inner = &closerOwner{closers: []io.Closer{file}}
	return nil
}

func returnedNestedOwner() (*closerOwner, error) {
	file, err := os.Open("state")
	if err != nil {
		return nil, err
	}
	return &closerOwner{closers: []io.Closer{file}}, nil
}

func returnedAppendedOwner(extra bool) (*closerOwner, error) {
	file, err := os.Open("state")
	if err != nil {
		return nil, err
	}
	closers := []io.Closer{file}
	if extra {
		closers = append(closers, io.NopCloser(bytes.NewReader(nil)))
	}
	return &closerOwner{closers: closers}, nil
}

func tickerOwnedByWorker(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	go func() {
		defer ticker.Stop()
		<-stop
	}()
}

func tickerStoppedOnWorkerExit(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	go func() {
		select {
		case <-ticker.C:
		case <-stop:
		}
		ticker.Stop()
	}()
}

func tickerConditionallyStoppedByWorker(stop <-chan struct{}, cleanup bool) {
	ticker := time.NewTicker(time.Second) // want "owned resource from time.NewTicker is not released on every return path"
	go func() {
		<-stop
		if cleanup {
			ticker.Stop()
		}
	}()
}

func fileTransferredToReceiver(files chan<- *os.File) error {
	file, err := os.Open("state")
	if err != nil {
		return err
	}
	files <- file
	return nil
}

func fileClosedByTestCleanup(t *testing.T) error {
	file, err := os.CreateTemp(t.TempDir(), "cleanup")
	if err != nil {
		return err
	}
	t.Cleanup(func() { _ = file.Close() })
	return nil
}

func fileConditionallyClosedByTestCleanup(t *testing.T, closeFile bool) error {
	file, err := os.CreateTemp(t.TempDir(), "conditional-cleanup") // want "owned resource from os.CreateTemp is not released on every return path"
	if err != nil {
		return err
	}
	t.Cleanup(func() {
		if closeFile {
			_ = file.Close()
		}
	})
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

func consumedTimer() {
	timer := time.NewTimer(time.Second)
	<-timer.C
}

func consumedTimerInSelect() {
	timer := time.NewTimer(time.Second)
	select {
	case <-timer.C:
	default:
	}
}

func unrelatedReceiveDoesNotConsumeTimer(events <-chan time.Time) {
	timer := time.NewTimer(time.Second) // want "owned resource from time.NewTimer is not released on every return path"
	<-events
	_ = timer
}

func transferredFile() (*os.File, error) {
	return os.CreateTemp("", "transfer")
}

var packageFile *os.File

func transferredFileToPackageOwner() error {
	file, err := os.Open("state")
	if err != nil {
		return err
	}
	packageFile = file
	return nil
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

func invokeCleanup(cleanup func() error) { _ = cleanup() }

func maybeInvokeCleanup(cleanup func() error, enabled bool) {
	if enabled {
		_ = cleanup()
	}
}

func observeCleanup(func() error) {}

func responseClosedByDeferredHelper(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer invokeCleanup(response.Body.Close)
	return nil
}

func conditionalDeferredHelperLeaksResponse(client *http.Client, request *http.Request, enabled bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	defer maybeInvokeCleanup(response.Body.Close, enabled)
	return nil
}

func nonDeferredObserverLeaksResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	observeCleanup(response.Body.Close)
	return nil
}

func deferredHelperClosesOnlyItsBoundResponse(client *http.Client, first, second *http.Request) error {
	closed, err := client.Do(first)
	if err != nil {
		return err
	}
	leaked, err := client.Do(second) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		_ = closed.Body.Close()
		return err
	}
	defer invokeCleanup(closed.Body.Close)
	_ = leaked
	return nil
}

func responseClosedByImmediateNestedDefer(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	func() {
		defer func() { _ = response.Body.Close() }()
	}()
	return nil
}

func responseConditionallyClosedByImmediateNestedDefer(client *http.Client, request *http.Request, closeBody bool) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	func() {
		if closeBody {
			defer func() { _ = response.Body.Close() }()
		}
	}()
	return nil
}

func responseCleanupClosureNotCalled(client *http.Client, request *http.Request) error {
	response, err := client.Do(request) // want "owned resource from http.Do is not released on every return path"
	if err != nil {
		return err
	}
	cleanup := func() { _ = response.Body.Close() }
	_ = cleanup
	return nil
}

func returnedResponseBody(client *http.Client, request *http.Request) (io.ReadCloser, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}

func responseOwnedByWorker(client *http.Client, request *http.Request) (<-chan struct{}, error) {
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer response.Body.Close()
	}()
	return done, nil
}

func conditionallyReturnedResponse(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if response != nil {
		_ = response.Body.Close()
	}
	return err
}

func stoppedOrConsumedTimer(events <-chan int) {
	timer := time.NewTimer(time.Second)
	done := false
	for !done {
		select {
		case <-events:
			done = true
		case <-timer.C:
			return
		}
	}
	if !timer.Stop() {
		<-timer.C
	}
}

func stoppedOrConsumedTimerAfterSeveralEvents(events <-chan string) {
	identities := make(map[string]bool, 2)
	timedOut := false
	timer := time.NewTimer(time.Second)
	for len(identities) < 2 && !timedOut {
		select {
		case identity := <-events:
			identities[identity] = true
		case <-timer.C:
			timedOut = true
		}
	}
	if !timedOut && !timer.Stop() {
		<-timer.C
	}
}

func partiallyHandledTimer(stop, receive bool) {
	timer := time.NewTimer(time.Second) // want "owned resource from time.NewTimer is not released on every return path"
	if stop {
		timer.Stop()
	}
	if receive {
		<-timer.C
	}
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

type fluentFileOwner struct{ file *os.File }

func (owner *fluentFileOwner) WithFile(file *os.File) *fluentFileOwner {
	owner.file = file
	return owner
}

func (owner *fluentFileOwner) Close() error { return owner.file.Close() }

func returnedFluentFileOwner() (*fluentFileOwner, error) {
	file, err := os.Open("fixture")
	if err != nil {
		return nil, err
	}
	owner := (&fluentFileOwner{}).WithFile(file)
	return owner, nil
}

func acquiredForEnclosingScope() func() error {
	var file *os.File
	load := func() error {
		var err error
		file, err = os.Open("fixture")
		return err
	}
	_ = load()
	return file.Close
}
