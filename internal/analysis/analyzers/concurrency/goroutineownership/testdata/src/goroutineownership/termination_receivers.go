package goroutineownership

import (
	"testing"

	"github.com/matryer/is"
)

func capturedStrictAssertion(t *testing.T, timeout <-chan struct{}) {
	assert := is.New(t)
	done := make(chan struct{})
	go func() { assert.NoErr(nil); close(done) }()
	select {
	case <-done:
	case <-timeout:
		assert.Fail()
	}
}

func capturedRelaxedAssertion(t *testing.T, timeout <-chan struct{}) {
	assert := is.NewRelaxed(t)
	done := make(chan struct{})
	go func() { assert.NoErr(nil); close(done) }() // want "goroutine is not joined on every return path"
	select {
	case <-done:
	case <-timeout:
		assert.Fail()
	}
}

func capturedMixedAssertion(t *testing.T, timeout <-chan struct{}, relaxed bool) {
	assert := is.New(t)
	if relaxed {
		assert = is.NewRelaxed(t)
	}
	done := make(chan struct{})
	go func() { assert.NoErr(nil); close(done) }() // want "goroutine is not joined on every return path"
	select {
	case <-done:
	case <-timeout:
		assert.Fail()
	}
}
