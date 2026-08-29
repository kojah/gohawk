package errorownership

import (
	"fmt"
	"log"
	"strings"
)

func logAndReturn(err error) error {
	log.Print(err) // want "error is logged and returned by same function"
	return err
}

func logAliasAndReturn(err error) error {
	alias := err
	log.Print(alias) // want "error is logged and returned by same function"
	return err
}

func logAndReturnWrapped(err error) error {
	log.Print(err) // want "error is logged and returned by same function"
	return fmt.Errorf("operation failed: %w", err)
}

func inspectText(err error) bool {
	return strings.Contains(err.Error(), "missing") // want "do not classify errors by matching Error text"
}

func inspectStoredText(err error) bool {
	message := err.Error()
	return strings.Contains(message, "missing") // want "do not classify errors by matching Error text"
}

func inspectConvertedText(err error) bool {
	message := []byte(err.Error())
	return strings.HasPrefix(string(message), "missing") // want "do not classify errors by matching Error text"
}

func inspectNestedText(err error) bool {
	message := strings.TrimSpace(err.Error())
	return strings.EqualFold(message, "missing") // want "do not classify errors by matching Error text"
}

type status string

func (value status) Error(code int) string { return string(value) }

func inspectNonErrorMethod(value status) bool {
	return strings.Contains(value.Error(0), "missing")
}

func inspectUnrelatedText(message string) bool {
	return strings.HasSuffix(message, "missing")
}

func legacyDirectiveDoesNotSuppress(err error) bool {
	//gohawk:error-text-match legacy directive is no longer supported
	return strings.Contains(err.Error(), "missing") // want "do not classify errors by matching Error text"
}

func exclusiveHandling(err error, returnError bool) error {
	if returnError {
		return err
	}
	log.Print(err)
	return nil
}

func consume(err error) {
	log.Print(err)
}

func logAndReturnText(value string) string {
	log.Print(value)
	return value
}

func regressionReadConfig() error { return nil }

func mismatchedInlineError(previousErr error) error {
	if err := regressionReadConfig(); previousErr != nil { // want "condition checks previousErr instead of newly declared err"
		return err
	}
	return nil
}

func matchedInlineError(previousErr error) error {
	if err := regressionReadConfig(); err != nil {
		return err
	}
	return previousErr
}

func intentionalValueCondition(previousErr error) error {
	if value, err := valueAndError(); value != "" {
		return err
	}
	return previousErr
}

func valueAndError() (string, error) { return "", nil }
