package synchelpers

import "sync"

func Receive(ch <-chan struct{})          { <-ch }
func Wait(group *sync.WaitGroup)          { group.Wait() }
func Forward(ch <-chan struct{})          { Receive(ch) }
func Other(first, second <-chan struct{}) { <-second }
func Maybe(ch <-chan struct{}, yes bool) {
	if yes {
		<-ch
	}
}
var hook func(<-chan struct{})

func Unknown(ch <-chan struct{}) { hook(ch) }
