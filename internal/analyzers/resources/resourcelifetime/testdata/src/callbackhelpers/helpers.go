// Package callbackhelpers forwards cleanup callbacks through a sibling
// helper, as ubuntu/decorate's LogFuncOnError does.
package callbackhelpers

import "log"

// LogOnError calls f through its sibling on every return.
func LogOnError(f func() error) {
	logOnErrorWith("prefix", f)
}

func logOnErrorWith(prefix string, f func() error) {
	if err := f(); err != nil {
		log.Print(prefix, err)
	}
}

// MaybeLogOnError skips f when quiet is set.
func MaybeLogOnError(quiet bool, f func() error) {
	if quiet {
		return
	}
	logOnErrorWith("prefix", f)
}
