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
