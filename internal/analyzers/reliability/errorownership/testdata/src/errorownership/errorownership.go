package errorownership

import (
	"fmt"
	"log"
	"log/slog"
	"strings"
)

func logAndReturn(err error) error {
	log.Print(err) // want "error is logged and returned by same function"
	return err
}

func logAndReturnWrapped(err error) error {
	log.Print(err) // want "error is logged and returned by same function"
	return fmt.Errorf("operation failed: %w", err)
}

func exclusiveHandling(err error, returnError bool) error {
	if returnError {
		return err
	}
	log.Print(err)
	return nil
}

func exclusiveDeferredLoopHandling(err error, handled bool) (any, error) {
	defer func() {}()
	var current error
	for i := 0; i < 2; i++ {
		current = err
		if strings.Contains(current.Error(), "handled") || handled {
			log.Printf("handled: %v", current)
			return nil, nil
		}
	}
	return nil, fmt.Errorf("operation failed: %w", current)
}

func sharedReturnPath(err error, handled bool) error {
	defer func() {}()
	if handled {
		log.Print(err) // want "error is logged and returned by same function"
	}
	return err
}

func predecessorLoadedLogAndReturn(err error, handled bool) (returned error) {
	defer func() {}()
	for handled {
		log.Print(err) // want "error is logged and returned by same function"
		returned = err
		break
	}
	return
}

func logVariadicAndReturn(err error) error {
	slog.Error("operation failed", "error", err) // want "error is logged and returned by same function"
	return err
}

func logDerivedErrorAndReturn(err error) error {
	log.Print(err.Error()) // want "error is logged and returned by same function"
	return err
}

func logFormattedErrorAndReturn(err error) error {
	log.Print(fmt.Sprintf("operation failed: %v", err)) // want "error is logged and returned by same function"
	return err
}

func errorCode(err error) int {
	if err == nil {
		return 0
	}
	return 1
}

func logErrorCodeAndReturn(err error) error {
	log.Print(errorCode(err)) // want "error is logged and returned by same function"
	return err
}

func deferredWrappedReturn(err error) (returned error) {
	defer func() {}()
	log.Print(err) // want "error is logged and returned by same function"
	returned = fmt.Errorf("operation failed: %w", err)
	return
}

type responsePayload struct{}

func (responsePayload) Error() string { return "successful response" }

func encodePayload(responsePayload) ([]byte, error) { return nil, nil }

func writePayload(responsePayload) error { return nil }

func readPayload(responsePayload) ([]byte, error) { return nil, nil }

func logAndReturnTupleError(payload responsePayload) error {
	_, err := readPayload(payload)
	log.Print(err) // want "error is logged and returned by same function"
	return err
}

func independentErrorsFromErrorImplementingPayload(payload responsePayload) error {
	_, encodeErr := encodePayload(payload)
	if encodeErr != nil {
		log.Print(encodeErr)
	}
	return writePayload(payload)
}

func firstProducer(any) error  { return nil }
func secondProducer(any) error { return nil }

func independentErrorsFromSharedInput(input any) error {
	firstErr := firstProducer(input)
	log.Print(firstErr)
	return secondProducer(input)
}

func independentDerivedErrors(input any) error {
	firstErr := firstProducer(input)
	log.Print(errorCode(firstErr))
	return secondProducer(input)
}

func phiRetainsErrorIdentity(first, second error, useFirst bool) error {
	returned := first
	if !useFirst {
		returned = second
	}
	log.Print(first) // want "error is logged and returned by same function"
	return returned
}

func opaqueWrap(err error) error { return err }

func projectWrapperIsOpaque(err error) error {
	log.Print(err)
	return opaqueWrap(err)
}

func nonWrappingErrorf(err error) error {
	log.Print(err)
	return fmt.Errorf("operation failed: %v", err)
}

func dynamicWrappingErrorf(err error, format string) error {
	log.Print(err)
	return fmt.Errorf(format, err)
}

func ambiguousWrappingErrorf(err error, payload responsePayload) error {
	log.Print(payload)
	return fmt.Errorf("operation failed: %w: %v", err, payload)
}

//gohawk:example flagged
func load(err error) error {
	log.Print(err) // want "error is logged and returned by same function"
	return err
}

//gohawk:example end

//gohawk:example ok
func loadWithContext(err error) error {
	return fmt.Errorf("load: %w", err)
}

//gohawk:example end
