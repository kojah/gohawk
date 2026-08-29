// Command measure runs a command and records its wall time, peak resident
// memory, and exit status for the dogfooding benchmark harness.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(arguments []string) int {
	flags := flag.NewFlagSet("measure", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	output := flags.String("output", "", "write tab-separated measurements to this file")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	commandArguments := flags.Args()
	if *output == "" || len(commandArguments) == 0 {
		fmt.Fprintln(os.Stderr, "usage: measure -output FILE -- COMMAND [ARG ...]")
		return 2
	}

	command := exec.Command(commandArguments[0], commandArguments[1:]...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	started := time.Now()
	err := command.Run()
	elapsed := time.Since(started)
	if command.ProcessState == nil {
		fmt.Fprintf(os.Stderr, "measure: start command: %v\n", err)
		return 1
	}

	peakRSS, rssErr := peakRSSKiB(command.ProcessState.SysUsage(), runtime.GOOS)
	if rssErr != nil {
		fmt.Fprintf(os.Stderr, "measure: inspect peak resident memory: %v\n", rssErr)
		return 1
	}
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			fmt.Fprintf(os.Stderr, "measure: run command: %v\n", err)
			return 1
		}
		exitCode = exitError.ExitCode()
	}

	measurement := fmt.Sprintf("%.6f\t%d\t%d\n", elapsed.Seconds(), peakRSS, exitCode)
	if err := os.WriteFile(*output, []byte(measurement), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "measure: write result: %v\n", err)
		return 1
	}
	return exitCode
}

// peakRSSKiB uses the Maxrss field exposed by os.ProcessState without tying
// the helper to one syscall.Rusage definition. Unix kernels report this value
// in KiB except Darwin, which reports bytes.
func peakRSSKiB(usage any, goos string) (int64, error) {
	value := reflect.ValueOf(usage)
	if !value.IsValid() {
		return 0, errors.New("process resource usage is unavailable")
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0, errors.New("process resource usage is unavailable")
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return 0, fmt.Errorf("unexpected resource usage type %T", usage)
	}
	maxRSS := value.FieldByName("Maxrss")
	if !maxRSS.IsValid() || !maxRSS.CanInt() {
		return 0, fmt.Errorf("resource usage type %T has no integer Maxrss field", usage)
	}
	result := maxRSS.Int()
	if goos == "darwin" {
		result /= 1024
	}
	return result, nil
}
