package docerrorownership

import (
	"fmt"
	"log"
)

func readConfig() error { return nil }

//gohawk:example flagged
func load() error {
	if err := readConfig(); err != nil {
		log.Print(err) // want "error is logged and returned by same function"
		return err
	}
	return nil
}

//gohawk:example end

//gohawk:example ok
func loadWithContext() error {
	if err := readConfig(); err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	return nil
}

//gohawk:example end
