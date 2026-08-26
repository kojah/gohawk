package resourcelifetime

import (
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
