// Package examples contains teaching examples, including intentional defects.
// These functions are compiled and analyzed for the talk; do not run them as a service.
package examples

import (
	"fmt"
	"io"
	"os"
)

// #region leaky
func printHeader(path string, dst io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	if _, err := io.CopyN(dst, file, 64); err != nil {
		return fmt.Errorf("copy header: %w", err)
	}
	return file.Close()
}

// #endregion leaky

// #region fixed
func printHeaderSafely(path string, dst io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.CopyN(dst, file, 64); err != nil {
		return fmt.Errorf("copy header: %w", err)
	}
	return nil
}

// #endregion fixed

// #region helpers
func closeFile(file *os.File) { _ = file.Close() }

func printViaHelper(path string, dst io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer closeFile(file)
	_, err = io.CopyN(dst, file, 64)
	return err
}

// #endregion helpers

// #region transfer
func openHeader(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return file, nil // The caller receives the file and its cleanup duty.
}

// #endregion transfer

// #region validation-leak
func openValidated(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	var header [4]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	return file, nil // Caller continues reading after the header.
}

// #endregion validation-leak

// #region validation-fixed
func openValidatedSafely(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	var header [4]byte
	if _, err := io.ReadFull(file, header[:]); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("read header: %w", err)
	}
	return file, nil // Caller now owns the open file.
}

// #endregion validation-fixed
