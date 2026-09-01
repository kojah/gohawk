package exitpolicy

import (
	"log"
	"os"
)

//gohawk:example flagged
func run() {
	file, _ := os.CreateTemp("", "state")
	defer file.Close()
	log.Fatal("startup failed") // want "log.Fatal exits without running an earlier defer"
}

//gohawk:example end

//gohawk:example ok
func runSafely() error {
	file, err := os.CreateTemp("", "state")
	if err != nil {
		return err
	}
	defer file.Close()
	return nil
}

//gohawk:example end
