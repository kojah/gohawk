// Package main covers workers that program exit stops. A goroutine launched
// at most once by main.main cannot accumulate, and every way out of main ends
// the process. A launch that can repeat is still reported.
package main

import "os"

func main() {
	done := make(chan struct{})
	go func() { close(done) }()

	for range os.Args {
		finished := make(chan struct{})
		go func() { close(finished) }() // want "goroutine is not joined on every return path"
	}

	start := func() {
		stopped := make(chan struct{})
		go func() { close(stopped) }() // want "goroutine is not joined on every return path"
	}
	start()
}
