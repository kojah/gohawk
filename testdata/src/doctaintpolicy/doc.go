package doctaintpolicy

import (
	"errors"
	"os"
	"os/exec"
)

func validateTool(tool string) (string, error) {
	if tool != "compiler" {
		return "", errors.New("unsupported tool")
	}
	return tool, nil
}

//gohawk:example flagged
func runConfiguredTool() error {
	return exec.Command(os.Getenv("TOOL")).Run() // want "untrusted data reaches process sink exec.Command"
}

//gohawk:example end

//gohawk:example ok
func runValidatedTool() error {
	tool, err := validateTool(os.Getenv("TOOL"))
	if err != nil {
		return err
	}
	return exec.Command(tool).Run()
}

//gohawk:example end
