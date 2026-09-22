package mixedcycles

import "effecthelpers"

func importedDifferentField() {
	var s effecthelpers.State
	done := make(chan struct{})
	s.Mu.Lock()
	go s.OtherFinish(done)
	<-done
	s.Mu.Unlock()
}

func importedFieldJoin() {
	var s effecthelpers.State
	done := make(chan struct{})
	s.Mu.Lock()
	go s.Finish(done)
	<-done // want "waiting for a worker while holding the mutex"
	s.Mu.Unlock()
}
