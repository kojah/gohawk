package goroutineownership

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

func ignoreAccessor() {
	w := newResultWorker()
	go w.Check() // want "goroutine is not joined on every return path"
}

func unrelatedResults() <-chan int { return make(chan int) }

func drainUnrelatedAccessor() {
	w := newResultWorker()
	go w.Check() // want "goroutine is not joined on every return path"
	for range unrelatedResults() {
	}
}
