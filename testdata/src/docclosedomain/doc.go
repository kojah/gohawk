package docclosedomain

//gohawk:example flagged
type Job struct {
	State string // want "field State uses a closed string domain" State:"closedStringDomain"
}

func finished(job Job) bool {
	return job.State == "done" || job.State == "failed"
}

//gohawk:example end

//gohawk:example ok
type TaskState string

const (
	TaskDone   TaskState = "done"
	TaskFailed TaskState = "failed"
)

type Task struct {
	State TaskState
}

//gohawk:example end
