package lockorder

import "sync"

// An acquisition on an object no other goroutine can reach yet orders
// nothing: locking a job's mutex while the registry lock is held, before the
// job is published into the registry, is initialization, not a lock order.
// The proof has two halves. In the function that locks, the object is
// unescaped at the acquisition; in every caller in the package, the argument
// is a fresh local that has not escaped at the call. Each accepted form
// below removes one half and stays reported.

type job struct {
	mu sync.Mutex
	id int
}

var (
	jobsMu sync.Mutex
	jobs   = map[int]*job{}
)

// addJob locks the job before publishing it; every caller hands it a fresh
// job, so the reverse edge is initialization and the steady-state order
// below is the only one.
func addJob(j *job) {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	j.mu.Lock()
	jobs[j.id] = j
	j.mu.Unlock()
}

func newJob(id int) {
	j := &job{id: id}
	addJob(j)
}

func steadyState(j *job) {
	j.mu.Lock()
	defer j.mu.Unlock()
	jobsMu.Lock()
	jobsMu.Unlock()
}

// The same initialization inline: the job is published after the lock.
func addJobInline(id int) {
	j := &job{id: id}
	jobsMu.Lock()
	defer jobsMu.Unlock()
	j.mu.Lock()
	jobs[j.id] = j
	j.mu.Unlock()
}

// Published first, then locked: the object may be contended, so the edge
// stands and the steady-state order closes a cycle.
type task struct {
	mu sync.Mutex
	id int
}

var (
	tasksMu sync.Mutex
	tasks   = map[int]*task{}
)

func addTaskPublished(t *task) {
	tasksMu.Lock()
	defer tasksMu.Unlock()
	tasks[t.id] = t
	t.mu.Lock()
	t.mu.Unlock()
}

func newTask(id int) {
	addTaskPublished(&task{id: id})
}

func taskSteadyState(t *task) {
	t.mu.Lock()
	defer t.mu.Unlock()
	tasksMu.Lock() // want "contradictory lock order: .*"
	tasksMu.Unlock()
}

// One caller passes an object it already holds: the parameter is not
// exclusively owned on entry.
type slot struct {
	mu sync.Mutex
	id int
}

var (
	slotsMu sync.Mutex
	slots   = map[int]*slot{}
	current *slot
)

func addSlot(s *slot) {
	slotsMu.Lock()
	defer slotsMu.Unlock()
	s.mu.Lock()
	slots[s.id] = s
	s.mu.Unlock()
}

func newSlot(id int) {
	addSlot(&slot{id: id})
}

func readdSlot() {
	addSlot(current)
}

func slotSteadyState(s *slot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slotsMu.Lock() // want "contradictory lock order: .*"
	slotsMu.Unlock()
}

// An exported function may be called from outside the package with any
// object, so its parameter is never proven exclusive.
type entry struct {
	mu sync.Mutex
	id int
}

var (
	entriesMu sync.Mutex
	entries   = map[int]*entry{}
)

func AddEntry(e *entry) {
	entriesMu.Lock()
	defer entriesMu.Unlock()
	e.mu.Lock()
	entries[e.id] = e
	e.mu.Unlock()
}

func newEntry(id int) {
	AddEntry(&entry{id: id})
}

func entrySteadyState(e *entry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	entriesMu.Lock() // want "contradictory lock order: .*"
	entriesMu.Unlock()
}

// The fresh object escaped through a goroutine before the lock, so it is
// not exclusive. Its lock identity is then unknown under the existing
// publication rule, which stays a coverage loss rather than a report.
type worker struct {
	mu sync.Mutex
	id int
}

var (
	workersMu sync.Mutex
	workers   = map[int]*worker{}
)

func (w *worker) run() {}

func addWorkerStarted(id int) {
	w := &worker{id: id}
	go w.run()
	workersMu.Lock()
	defer workersMu.Unlock()
	w.mu.Lock()
	workers[w.id] = w
	w.mu.Unlock()
}

func workerSteadyState(w *worker) {
	w.mu.Lock()
	defer w.mu.Unlock()
	workersMu.Lock()
	workersMu.Unlock()
}
