package resourcelifetime

import (
	"errors"
	"log"
	"os"
)

func loggerDiscardedOnError(path string, fail bool) (*log.Logger, error) {
	file, err := os.Create(path) // want "owned resource from os.Create is not released"
	if err != nil {
		return nil, err
	}
	logger := log.New(file, "", 0)
	if fail {
		return nil, errors.New("failed")
	}
	return logger, nil
}

func returnedLoggerAggregate(path string) (struct{ Logger *log.Logger }, error) {
	var result struct{ Logger *log.Logger }
	file, err := os.Create(path)
	if err != nil {
		return result, err
	}
	result.Logger = log.New(file, "", 0)
	return result, nil
}

func returnLoggerAggregateBeforeConstruction(path string, fail bool) (struct{ Logger *log.Logger }, error) {
	var result struct{ Logger *log.Logger }
	file, err := os.Create(path) // want "owned resource from os.Create is not released"
	if err != nil {
		return result, err
	}
	if fail {
		return result, errors.New("failed before assigning logger")
	}
	result.Logger = log.New(file, "", 0)
	return result, nil
}

func returnLogger(path string) (*log.Logger, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return log.New(file, "", 0), nil
}

func discardLogger(path string) error {
	file, err := os.Create(path) // want "owned resource from os.Create is not released"
	if err != nil {
		return err
	}
	_ = log.New(file, "", 0)
	return nil
}

func localLogger(path string) error {
	file, err := os.Create(path) // want "owned resource from os.Create is not released"
	if err != nil {
		return err
	}
	logger := log.New(file, "", 0)
	logger.Print("message")
	return nil
}

func returnUnrelatedLogger(path string) (*log.Logger, error) {
	file, err := os.Create(path) // want "owned resource from os.Create is not released"
	if err != nil {
		return nil, err
	}
	_ = log.New(file, "", 0)
	return log.New(os.Stderr, "", 0), nil
}
