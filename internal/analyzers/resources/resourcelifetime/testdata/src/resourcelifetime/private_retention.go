package resourcelifetime

import (
	"io"
	"log"
	"os"
)

var privateLogger *log.Logger

func installPrivateLogger(writer io.Writer) { privateLogger = log.New(writer, "", 0) }

func discardPrivateLogger(writer io.Writer) { _ = log.New(writer, "", 0) }

func maybeInstallPrivateLogger(writer io.Writer, fail bool) {
	logger := log.New(writer, "", 0)
	if fail {
		return
	}
	privateLogger = logger
}

func privateLoggerRetainsFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	installPrivateLogger(file)
}

func privateLoggerDiscardsFile(path string) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	discardPrivateLogger(file)
}

func privateLoggerMayDiscardFile(path string, fail bool) {
	file, err := os.Open(path) // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	maybeInstallPrivateLogger(file, fail)
}
