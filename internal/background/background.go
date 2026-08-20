// Package background runs bounded fleets of detached Neko agents.
//
// A background agent owns its own session, tool registry, and permission
// policy. It never touches the foreground conversation and never opens an
// interactive prompt, so a fleet cannot deadlock the terminal UI.
package background

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/m1neroma/neko/internal/core"
)

// MaxAgents is the hard ceiling on concurrent background agents.
const MaxAgents = 25

const (
	maxOutputLines = 500
	maxLineLength  = 4000
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusDone      Status = "done"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// Runner is the subset of the coding agent the manager drives.
type Runner interface {
	Run(ctx context.Context, prompt string) error
}

// Job is one prepared background agent plus the state it reports about itself.
type Job struct {
	Runner  Runner
	Session string
	Mode    string
}

// Factory builds an agent for a task. The writer captures everything the agent
// would otherwise print to the terminal.
type Factory func(id string, out *Writer) (Job, error)

// Event reports a finished task to the foreground.
type Event struct {
	ID     string
	Label  string
	Status Status
	Err    string
}

// Snapshot is an immutable view of one task.
type Snapshot struct {
	ID         string
	Label      string
	Prompt     string
	Session    string
	Mode       string
	Status     Status
	Err        string
	CreatedAt  time.Time
	FinishedAt time.Time
	Lines      int
}

// Duration reports elapsed time for running tasks and total time for finished ones.
func (s Snapshot) Duration() time.Duration {
	if s.FinishedAt.IsZero() {
		return time.Since(s.CreatedAt)
	}
	return s.FinishedAt.Sub(s.CreatedAt)
}

type entry struct {
	id         string
	label      string
	prompt     string
	session    string
	mode       string
	status     Status
	err        string
	createdAt  time.Time
	finishedAt time.Time
	output     []string
	dropped    int
	cancelled  bool
	cancel     context.CancelFunc
}

func (e *entry) snapshot() Snapshot {
	return Snapshot{
		ID: e.id, Label: e.label, Prompt: e.prompt, Session: e.session, Mode: e.mode,
		Status: e.status, Err: e.err, CreatedAt: e.createdAt, FinishedAt: e.finishedAt,
		Lines: len(e.output) + e.dropped,
	}
}

// Manager owns every background agent for one Neko session.
type Manager struct {
	mu      sync.Mutex
	limit   int
	counter int
	tasks   map[string]*entry
	order   []string
	factory Factory
	notify  func(Event)
	wg      sync.WaitGroup
}

// New creates a manager. The limit is clamped to 1..MaxAgents.
func New(limit int, factory Factory, notify func(Event)) *Manager {
	if limit <= 0 || limit > MaxAgents {
		limit = MaxAgents
	}
	return &Manager{limit: limit, tasks: map[string]*entry{}, factory: factory, notify: notify}
}

// Limit is the maximum number of agents that may run at once.
func (m *Manager) Limit() int { return m.limit }

// Running counts agents currently executing.
func (m *Manager) Running() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runningLocked()
}

func (m *Manager) runningLocked() int {
	count := 0
	for _, item := range m.tasks {
		if item.status == StatusRunning {
			count++
		}
	}
	return count
}

// Spawn starts a detached agent for prompt. It fails when the fleet is full.
func (m *Manager) Spawn(parent context.Context, prompt string) (Snapshot, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return Snapshot{}, errors.New("a task description is required")
	}
	if m.factory == nil {
		return Snapshot{}, errors.New("background agents are not configured")
	}
	if err := parent.Err(); err != nil {
		return Snapshot{}, err
	}
	m.mu.Lock()
	if running := m.runningLocked(); running >= m.limit {
		m.mu.Unlock()
		return Snapshot{}, fmt.Errorf("background limit reached: %d of %d agents already running", running, m.limit)
	}
	m.counter++
	id := fmt.Sprintf("bg-%d", m.counter)
	ctx, cancel := context.WithCancel(parent)
	item := &entry{
		id: id, label: Label(prompt), prompt: prompt, status: StatusRunning,
		createdAt: time.Now(), cancel: cancel,
	}
	m.tasks[id] = item
	m.order = append(m.order, id)
	m.mu.Unlock()

	writer := &Writer{manager: m, id: id}
	job, err := m.factory(id, writer)
	if err != nil {
		cancel()
		m.mu.Lock()
		delete(m.tasks, id)
		m.order = removeID(m.order, id)
		m.mu.Unlock()
		return Snapshot{}, err
	}
	m.mu.Lock()
	item.session, item.mode = job.Session, job.Mode
	snapshot := item.snapshot()
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancel()
		runErr := job.Runner.Run(ctx, prompt)
		writer.EndStream()
		m.finish(id, runErr)
	}()
	return snapshot, nil
}

func (m *Manager) finish(id string, runErr error) {
	m.mu.Lock()
	item, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	item.finishedAt = time.Now()
	switch {
	case item.cancelled || errors.Is(runErr, context.Canceled):
		item.status = StatusCancelled
		item.err = ""
	case runErr != nil:
		item.status = StatusFailed
		item.err = runErr.Error()
	default:
		item.status = StatusDone
	}
	event := Event{ID: item.id, Label: item.label, Status: item.status, Err: item.err}
	notify := m.notify
	m.mu.Unlock()
	if notify != nil {
		notify(event)
	}
}

// List returns every task in spawn order.
func (m *Manager) List() []Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Snapshot, 0, len(m.order))
	for _, id := range m.order {
		if item, ok := m.tasks[id]; ok {
			out = append(out, item.snapshot())
		}
	}
	return out
}

// Get returns one task.
func (m *Manager) Get(id string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.tasks[id]
	if !ok {
		return Snapshot{}, fmt.Errorf("unknown background agent %q", id)
	}
	return item.snapshot(), nil
}

// Output returns the captured transcript of one task.
func (m *Manager) Output(id string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.tasks[id]
	if !ok {
		return nil, fmt.Errorf("unknown background agent %q", id)
	}
	out := make([]string, 0, len(item.output)+1)
	if item.dropped > 0 {
		out = append(out, fmt.Sprintf("… %d earlier lines dropped …", item.dropped))
	}
	return append(out, item.output...), nil
}

// Cancel stops one running task.
func (m *Manager) Cancel(id string) (Snapshot, error) {
	m.mu.Lock()
	item, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return Snapshot{}, fmt.Errorf("unknown background agent %q", id)
	}
	if item.status != StatusRunning {
		snapshot := item.snapshot()
		m.mu.Unlock()
		return snapshot, fmt.Errorf("agent %s already %s", id, snapshot.Status)
	}
	item.cancelled = true
	cancel := item.cancel
	snapshot := item.snapshot()
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return snapshot, nil
}

// CancelAll stops every running task and reports how many were signalled.
func (m *Manager) CancelAll() int {
	m.mu.Lock()
	var cancels []context.CancelFunc
	for _, item := range m.tasks {
		if item.status == StatusRunning {
			item.cancelled = true
			if item.cancel != nil {
				cancels = append(cancels, item.cancel)
			}
		}
	}
	m.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	return len(cancels)
}

// Shutdown cancels every task and waits up to timeout for them to unwind.
func (m *Manager) Shutdown(timeout time.Duration) int {
	stopped := m.CancelAll()
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
	return stopped
}

func (m *Manager) append(id, text string) {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.tasks[id]
	if !ok {
		return
	}
	for _, line := range strings.Split(text, "\n") {
		if len(line) > maxLineLength {
			line = line[:maxLineLength] + "…"
		}
		item.output = append(item.output, line)
	}
	if overflow := len(item.output) - maxOutputLines; overflow > 0 {
		item.output = append([]string(nil), item.output[overflow:]...)
		item.dropped += overflow
	}
}

// Writer captures agent progress for one background task. It satisfies both the
// agent UI and the tool reporter interfaces.
type Writer struct {
	manager *Manager
	id      string
	mu      sync.Mutex
	stream  strings.Builder
}

// Thinking is a no-op: background agents have no spinner.
func (w *Writer) Thinking() {}

// Stream buffers assistant text until the turn ends.
func (w *Writer) Stream(text string) {
	w.mu.Lock()
	w.stream.WriteString(text)
	w.mu.Unlock()
}

// EndStream flushes buffered assistant text into the transcript.
func (w *Writer) EndStream() {
	w.mu.Lock()
	text := strings.TrimSpace(w.stream.String())
	w.stream.Reset()
	w.mu.Unlock()
	if text != "" {
		w.manager.append(w.id, text)
	}
}

// Tool records that a tool started.
func (w *Writer) Tool(name, detail string) {
	line := "● " + name
	if detail != "" {
		line += "(" + detail + ")"
	}
	w.manager.append(w.id, line)
}

// ToolResult records a one-line tool outcome.
func (w *Writer) ToolResult(name, result string, failed bool) {
	prefix := "└─ "
	if failed {
		prefix = "└─ error: "
	}
	w.manager.append(w.id, prefix+firstLine(result))
}

// Info records an informational line.
func (w *Writer) Info(text string) { w.manager.append(w.id, "● "+text) }

// Ask refuses: a detached agent has nobody to answer it.
func (w *Writer) Ask(question string, options []core.QuestionOption) (string, error) {
	w.manager.append(w.id, "? "+question+" (unanswered: background agent)")
	return "", errors.New("background agents cannot ask clarification questions; run this task in the foreground")
}

// Label shortens a prompt into a status-line friendly title.
func Label(prompt string) string {
	line := strings.TrimSpace(strings.Split(strings.TrimSpace(prompt), "\n")[0])
	runes := []rune(line)
	if len(runes) > 48 {
		return string(runes[:48]) + "…"
	}
	if line == "" {
		return "background task"
	}
	return line
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "completed"
	}
	line := strings.TrimSpace(strings.Split(value, "\n")[0])
	runes := []rune(line)
	if len(runes) > 120 {
		line = string(runes[:120]) + "…"
	}
	if count := strings.Count(value, "\n") + 1; count > 1 {
		line += fmt.Sprintf(" (%d lines)", count)
	}
	return line
}

func removeID(ids []string, id string) []string {
	for i, current := range ids {
		if current == id {
			return append(ids[:i], ids[i+1:]...)
		}
	}
	return ids
}
