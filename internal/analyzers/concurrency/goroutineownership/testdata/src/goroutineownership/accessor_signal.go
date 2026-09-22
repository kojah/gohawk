package goroutineownership

// Unjoined workers whose completion channel is obtained through a constructor
// remain an accepted coverage gap: the factory may retain another join owner.

type resultWorker struct{ results chan int }

func newResultWorker() *resultWorker        { return &resultWorker{results: make(chan int)} }
func (w *resultWorker) Results() <-chan int { return w.results }
func (w *resultWorker) Check()              { close(w.results) }

func drainAccessor() {
	w := newResultWorker()
	go w.Check()
	for range w.Results() {
	}
}
