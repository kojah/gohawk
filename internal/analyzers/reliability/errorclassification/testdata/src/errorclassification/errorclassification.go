package errorclassification

import (
	"os/exec"
	"strings"
)

//gohawk:example flagged
func missing(err error) bool {
	return strings.Contains(err.Error(), "missing") // want "do not classify errors by matching Error text"
}

//gohawk:example end

//gohawk:example ok
func unrelated(message string) bool {
	return strings.Contains(message, "missing")
}

//gohawk:example end

func transformed(err error) bool {
	message := strings.TrimSpace(err.Error())
	return strings.EqualFold(message, "missing") // want "do not classify errors by matching Error text"
}

func externalCommand() bool {
	err := exec.Command("fixture").Run()
	return err != nil && strings.Contains(err.Error(), "missing")
}
