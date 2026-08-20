package background

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type stubRunner struct {
	fn func(ctx context.Context, prompt string) error
}

func (s stubRunner) Run(ctx context.Context, prompt string) error { return s.fn(ctx, prompt) }

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition was not met before the timeout")
}

func TestSpawnRunsDetachedAndReportsCompletion(t *testing.T) {
	events := make(chan Event, 1)
	manager := New(MaxAgents, func(id string, out *Writer) (Job, error) {
		return Job{Session: "session-" + id, Mode: "build", Runner: stubRunner{fn: func(ctx context.Context, prompt string) error {
			out.Tool("read_file", "main.go")
			out.ToolResult("read_file", "package main\nfunc main() {}", false)
			out.Stream("done with ")
			out.Stream(prompt)
			return nil
		}}}, nil
	}, func(event Event) { events <- event })

	snapshot, err := manager.Spawn(context.Background(), "fix the build")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ID != "bg-1" || snapshot.Status != StatusRunning || snapshot.Session != "session-bg-1" {
		t.Fatalf("unexpected snapshot %#v", snapshot)
	}

	event := <-events
	if event.Status != StatusDone || event.ID != "bg-1" || event.Label != "fix the build" {
		t.Fatalf("unexpected event %#v", event)
	}
	if running := manager.Running(); running != 0 {
		t.Fatalf("expected no running agents, got %d", running)
	}
	lines, err := manager.Output("bg-1")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"● read_file(main.go)", "└─ package main (2 lines)", "done with fix the build"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in output:\n%s", want, joined)
		}
	}
}

func TestSpawnEnforcesTwentyFiveAgentLimit(t *testing.T) {
	release := make(chan struct{})
	manager := New(0, func(id string, out *Writer) (Job, error) {
		return Job{Session: id, Mode: "build", Runner: stubRunner{fn: func(ctx context.Context, prompt string) error {
			<-release
			return nil
		}}}, nil
	}, nil)
	if manager.Limit() != MaxAgents {
		t.Fatalf("expected the default limit to be %d, got %d", MaxAgents, manager.Limit())
	}

	for i := 0; i < MaxAgents; i++ {
		if _, err := manager.Spawn(context.Background(), fmt.Sprintf("task %d", i)); err != nil {
			t.Fatalf("spawn %d failed: %v", i, err)
		}
	}
	waitFor(t, func() bool { return manager.Running() == MaxAgents })
	if _, err := manager.Spawn(context.Background(), "one too many"); err == nil {
		t.Fatal("expected the 26th agent to be rejected")
	} else if !strings.Contains(err.Error(), "25 of 25") {
		t.Fatalf("unexpected limit error %v", err)
	}
	close(release)
	waitFor(t, func() bool { return manager.Running() == 0 })

	// A finished slot is reusable.
	if _, err := manager.Spawn(context.Background(), "after drain"); err != nil {
		t.Fatalf("spawn after drain failed: %v", err)
	}
}

func TestCustomLimitIsClampedToMax(t *testing.T) {
	if limit := New(3, nil, nil).Limit(); limit != 3 {
		t.Fatalf("expected limit 3, got %d", limit)
	}
	if limit := New(999, nil, nil).Limit(); limit != MaxAgents {
		t.Fatalf("expected limit clamped to %d, got %d", MaxAgents, limit)
	}
}

func TestCancelMarksAgentCancelled(t *testing.T) {
	events := make(chan Event, 1)
	manager := New(MaxAgents, func(id string, out *Writer) (Job, error) {
		return Job{Session: id, Mode: "build", Runner: stubRunner{fn: func(ctx context.Context, prompt string) error {
			<-ctx.Done()
			return ctx.Err()
		}}}, nil
	}, func(event Event) { events <- event })

	if _, err := manager.Spawn(context.Background(), "long task"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Cancel("bg-1"); err != nil {
		t.Fatal(err)
	}
	event := <-events
	if event.Status != StatusCancelled || event.Err != "" {
		t.Fatalf("unexpected event %#v", event)
	}
	if _, err := manager.Cancel("bg-1"); err == nil {
		t.Fatal("expected a second cancel to report the agent is no longer running")
	}
	if _, err := manager.Cancel("bg-404"); err == nil {
		t.Fatal("expected an unknown agent to fail")
	}
}

func TestFailureIsRecorded(t *testing.T) {
	events := make(chan Event, 1)
	manager := New(MaxAgents, func(id string, out *Writer) (Job, error) {
		return Job{Session: id, Mode: "build", Runner: stubRunner{fn: func(ctx context.Context, prompt string) error {
			return errors.New("provider returned HTTP 500")
		}}}, nil
	}, func(event Event) { events <- event })

	if _, err := manager.Spawn(context.Background(), "failing task"); err != nil {
		t.Fatal(err)
	}
	event := <-events
	if event.Status != StatusFailed || event.Err != "provider returned HTTP 500" {
		t.Fatalf("unexpected event %#v", event)
	}
	snapshot, err := manager.Get("bg-1")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != StatusFailed || snapshot.FinishedAt.IsZero() {
		t.Fatalf("unexpected snapshot %#v", snapshot)
	}
}

func TestFactoryErrorReleasesTheSlot(t *testing.T) {
	manager := New(MaxAgents, func(id string, out *Writer) (Job, error) {
		return Job{}, errors.New("no active model; use /model")
	}, nil)
	if _, err := manager.Spawn(context.Background(), "task"); err == nil {
		t.Fatal("expected the factory error to surface")
	}
	if items := manager.List(); len(items) != 0 {
		t.Fatalf("expected no tracked agents, got %#v", items)
	}
	if running := manager.Running(); running != 0 {
		t.Fatalf("expected no running agents, got %d", running)
	}
}

func TestSpawnRejectsEmptyPrompt(t *testing.T) {
	manager := New(MaxAgents, func(id string, out *Writer) (Job, error) { return Job{}, nil }, nil)
	if _, err := manager.Spawn(context.Background(), "   \n  "); err == nil {
		t.Fatal("expected an empty prompt to be rejected")
	}
}

func TestWriterAskAlwaysFails(t *testing.T) {
	manager := New(MaxAgents, nil, nil)
	manager.tasks["bg-1"] = &entry{id: "bg-1", status: StatusRunning}
	manager.order = append(manager.order, "bg-1")
	writer := &Writer{manager: manager, id: "bg-1"}
	if _, err := writer.Ask("Which approach?", nil); err == nil {
		t.Fatal("expected background agents to refuse clarification questions")
	}
	lines, err := manager.Output("bg-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "Which approach?") {
		t.Fatalf("unexpected output %#v", lines)
	}
}

func TestOutputIsBoundedAndReportsDroppedLines(t *testing.T) {
	manager := New(MaxAgents, nil, nil)
	manager.tasks["bg-1"] = &entry{id: "bg-1", status: StatusRunning}
	manager.order = append(manager.order, "bg-1")
	for i := 0; i < maxOutputLines+10; i++ {
		manager.append("bg-1", fmt.Sprintf("line %d", i))
	}
	lines, err := manager.Output("bg-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != maxOutputLines+1 {
		t.Fatalf("expected %d lines, got %d", maxOutputLines+1, len(lines))
	}
	if !strings.Contains(lines[0], "10 earlier lines dropped") {
		t.Fatalf("unexpected first line %q", lines[0])
	}
	if lines[1] != "line 10" {
		t.Fatalf("unexpected retained line %q", lines[1])
	}
	long := strings.Repeat("x", maxLineLength+50)
	manager.append("bg-1", long)
	lines, _ = manager.Output("bg-1")
	last := lines[len(lines)-1]
	if len([]rune(last)) != maxLineLength+1 {
		t.Fatalf("expected the long line to be truncated, got %d runes", len([]rune(last)))
	}
}

func TestShutdownCancelsEveryRunningAgent(t *testing.T) {
	manager := New(MaxAgents, func(id string, out *Writer) (Job, error) {
		return Job{Session: id, Mode: "build", Runner: stubRunner{fn: func(ctx context.Context, prompt string) error {
			<-ctx.Done()
			return ctx.Err()
		}}}, nil
	}, nil)
	for i := 0; i < 3; i++ {
		if _, err := manager.Spawn(context.Background(), fmt.Sprintf("task %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if stopped := manager.Shutdown(5 * time.Second); stopped != 3 {
		t.Fatalf("expected 3 agents to be stopped, got %d", stopped)
	}
	for _, item := range manager.List() {
		if item.Status != StatusCancelled {
			t.Fatalf("expected %s to be cancelled, got %s", item.ID, item.Status)
		}
	}
}

func TestParentCancellationIsRefused(t *testing.T) {
	manager := New(MaxAgents, func(id string, out *Writer) (Job, error) { return Job{}, nil }, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.Spawn(ctx, "task"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected a cancelled parent context to be refused, got %v", err)
	}
}

func TestLabelShortensMultilinePrompts(t *testing.T) {
	if got := Label("  refactor the parser\nand add tests  "); got != "refactor the parser" {
		t.Fatalf("unexpected label %q", got)
	}
	long := Label(strings.Repeat("ы", 80))
	if len([]rune(long)) != 49 {
		t.Fatalf("expected a 48-rune label plus ellipsis, got %d runes", len([]rune(long)))
	}
	if got := Label("   "); got != "background task" {
		t.Fatalf("unexpected fallback label %q", got)
	}
}

func TestListPreservesSpawnOrder(t *testing.T) {
	manager := New(MaxAgents, func(id string, out *Writer) (Job, error) {
		return Job{Session: id, Mode: "plan", Runner: stubRunner{fn: func(ctx context.Context, prompt string) error { return nil }}}, nil
	}, nil)
	for i := 0; i < 4; i++ {
		if _, err := manager.Spawn(context.Background(), fmt.Sprintf("task %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, func() bool { return manager.Running() == 0 })
	items := manager.List()
	if len(items) != 4 {
		t.Fatalf("expected 4 agents, got %d", len(items))
	}
	for i, item := range items {
		if item.ID != fmt.Sprintf("bg-%d", i+1) || item.Label != fmt.Sprintf("task %d", i) {
			t.Fatalf("unexpected ordering at %d: %#v", i, item)
		}
		if item.Mode != "plan" {
			t.Fatalf("expected the inherited plan mode, got %q", item.Mode)
		}
	}
}
