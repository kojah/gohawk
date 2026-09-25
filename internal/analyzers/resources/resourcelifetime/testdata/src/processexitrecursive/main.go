// Package main calls its own main again, so main can run more than once and
// its acquisitions can accumulate.
package main

import "os"

func main() {
	file, err := os.Open("config") // want "owned resource from os.Open is not released"
	if err != nil {
		return
	}
	_ = file.Name()
	if len(os.Args) > 1 {
		os.Args = os.Args[1:]
		main()
	}
}
