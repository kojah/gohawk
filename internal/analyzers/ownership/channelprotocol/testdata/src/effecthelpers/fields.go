package effecthelpers

import "sync"

type State struct{ Mu, Other sync.Mutex }

func (s *State) Finish(done chan struct{})      { s.Mu.Lock(); s.Mu.Unlock(); close(done) }
func (s *State) OtherFinish(done chan struct{}) { s.Other.Lock(); s.Other.Unlock(); close(done) }

func SameBranch(ch chan int, flag bool) {
	if flag {
		ch <- 1
	} else {
		ch <- 2
	}
}

func BranchWorker(result, done chan int, flag bool) { SameBranch(result, flag); close(done) }
