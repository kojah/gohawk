package transferlabels

import "github.com/golang/mock/gomock"

type owner struct{ done chan bool }
type destination struct {
	done chan bool
	box  owner
}

func mixedReturn(choice bool) <-chan bool {
	done := make(chan bool)
	go func() { done <- true }()
	result := done
	if choice {
		result = make(chan bool)
	}
	return result
}

func overwrittenAggregate() owner {
	done := make(chan bool)
	go func() { done <- true }()
	result := owner{done: done}
	result.done = make(chan bool)
	return result
}

func discard(done chan bool) struct{} { return struct{}{} }

func discardedWrapper() struct{} {
	done := make(chan bool)
	go func() { done <- true }()
	return discard(done)
}

func exactStore(target *destination) {
	done := make(chan bool)
	go func() { done <- true }()
	target.done = done
}

func mixedStore(target *destination, choice bool) {
	done := make(chan bool)
	go func() { done <- true }()
	result := done
	if choice {
		result = make(chan bool)
	}
	target.done = result
}

func overwrittenAggregateStore(target *destination) {
	done := make(chan bool)
	go func() { done <- true }()
	result := owner{done: done}
	result.done = make(chan bool)
	target.box = result
}

func exactBesideOpaque() (owner, <-chan bool) {
	done := make(chan bool)
	go func() { done <- true }()
	result := owner{done: done}
	result.done = make(chan bool)
	return result, done
}

func configuredMockResult(call *gomock.Call) {
	done := make(chan bool)
	go func() { done <- true }()
	call.Return(done)
}
