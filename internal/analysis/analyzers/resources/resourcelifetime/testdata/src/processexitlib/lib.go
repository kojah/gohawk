// Package processexitlib has the same shapes as a program's main function,
// but library code can run many times, so process exit does not settle them.
package processexitlib

import "os"

func Run() {
	file, err := os.Open("config") // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	_ = file.Name()
}

// A function named main outside package main is not the program entry.
func main() {
	file, err := os.Open("config") // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	_ = file.Name()
}

var _ = main
