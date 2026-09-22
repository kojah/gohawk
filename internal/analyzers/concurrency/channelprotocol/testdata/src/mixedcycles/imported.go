package mixedcycles

import (
	"effectforward"
	"effecthelpers"
	"sync"
)

func importedJoin() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go effectforward.Worker(done, &mu)
	effecthelpers.Wait(done) // want "waiting for a worker while holding the mutex"
	effecthelpers.Unlock(&mu)
}

func importedGroupJoin() {
	var mu sync.Mutex
	var group sync.WaitGroup
	effecthelpers.AddOne(&group)
	mu.Lock()
	go effecthelpers.GroupWorker(&mu, &group)
	effecthelpers.Join(&group) // want "waiting for a worker while holding the mutex"
	mu.Unlock()
}

func importedChannelCycle() {
	var mu sync.Mutex
	ch := make(chan int)
	mu.Lock()
	go effecthelpers.Receive(&mu, ch)
	ch <- effecthelpers.Pure(1) // want "channel operation holds the mutex"
	mu.Unlock()
}

func importedExistingCycle() {
	ch, done := make(chan int), make(chan struct{})
	go effecthelpers.SendResult(ch, done)
	<-done // want "channel wait prevents"
	<-ch
}

func importedEarlySignal() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go effecthelpers.Early(done, &mu)
	<-done
	mu.Unlock()
}

func importedUnknown(skip bool) {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	go effecthelpers.Conditional(&mu, done, skip)
	<-done
	mu.Unlock()
}

func importedReset() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	effecthelpers.Reset(&mu)
	go effecthelpers.Worker(&mu, done)
	<-done
	mu.Unlock()
}

func importedExtraParticipant() {
	var mu sync.Mutex
	done := make(chan struct{})
	mu.Lock()
	effecthelpers.Launch(done)
	go effecthelpers.Worker(&mu, done)
	<-done
	mu.Unlock()
}
