package lockorder

import "sync"

// A private registration helper has one fresh synchronous caller, but also
// escapes as a callback. The direct caller cannot establish exclusive ownership
// for objects passed by the callback's users. The direct-only accepted form
// remains beside this boundary in exclusive_owners.go (addJob/newJob).
type callbackJob struct {
	mu sync.Mutex
}

var (
	callbackJobsMu sync.Mutex
	callbackJobs   []*callbackJob
	register      = registerCallbackJob
)

func registerCallbackJob(job *callbackJob) {
	callbackJobsMu.Lock()
	defer callbackJobsMu.Unlock()
	job.mu.Lock()
	callbackJobs = append(callbackJobs, job)
	job.mu.Unlock()
}

func createCallbackJob() {
	registerCallbackJob(&callbackJob{})
}

func useCallbackJob(job *callbackJob) {
	job.mu.Lock()
	defer job.mu.Unlock()
	callbackJobsMu.Lock() // want "contradictory lock order: .*"
	callbackJobsMu.Unlock()
}
