package errorownership

import (
	"fmt"
	"log"
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
