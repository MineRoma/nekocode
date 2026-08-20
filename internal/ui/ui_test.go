package ui

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

func TestCommandMatches(t *testing.T) {
	matches := commandMatches("/co", 4)
	if len(matches) != 4 {
		t.Fatalf("expected command matches, got %#v", matches)
	}
	if matches[0] != "/compact" || matches[1] != "/context" || matches[2] != "/cost" || matches[3] != "/checkpoint" {
		t.Fatalf("unexpected matches %#v", matches)
	}
	if got := commandMatches("hello", 4); got != nil {
		t.Fatalf("plain input should not suggest commands: %#v", got)
	}
}

func TestStatusUsesCapitalizedMode(t *testing.T) {
	model := newChatModel("build", false, "test-model", "session", t.TempDir(), make(chan Input, 2), false)
	status := model.statusView()
	if !strings.Contains(status, "Build") || strings.Contains(status, "build") {
		t.Fatalf("unexpected Build status %q", status)
	}
	model.mode = "plan"
	if status = model.statusView(); !strings.Contains(status, "Plan") || strings.Contains(status, "plan") {
		t.Fatalf("unexpected Plan status %q", status)
	}
	model.mode = "reverse"
	if status = model.statusView(); !strings.Contains(status, "Reverse") || strings.Contains(status, "reverse") {
		t.Fatalf("unexpected Reverse status %q", status)
	}
}

func TestAccentColorPerMode(t *testing.T) {
	model := newChatModel("build", false, "m", "s", t.TempDir(), make(chan Input, 1), true)
	build := model.accentColor()
	model.mode = "plan"
	plan := model.accentColor()
	model.mode = "reverse"
	reverse := model.accentColor()
	if build == plan || plan == reverse || build == reverse {
		t.Fatalf("each mode needs a distinct accent: build=%v plan=%v reverse=%v", build, plan, reverse)
	}
	// An unknown mode falls back to the Build accent rather than an empty color.
	model.mode = "nonsense"
	if model.accentColor() != build {
		t.Fatalf("unknown mode should reuse the build accent, got %v", model.accentColor())
	}
}

func TestReplayRendersHistoryBeforeLiveLogs(t *testing.T) {
	model := newChatModel("build", false, "m", "s", t.TempDir(), make(chan Input, 1), false)
	model.logs = append(model.logs, logEntry{kind: "info", text: "● live line"})
	updated, _ := model.Update(replayMsg{entries: []logEntry{
		{kind: "user", text: "❯ earlier question"},
		{kind: "assistant", text: "✦ earlier answer"},
	}})
	result := updated.(chatModel)
	if len(result.logs) != 3 {
		t.Fatalf("expected three entries, got %#v", result.logs)
	}
	if result.logs[0].text != "❯ earlier question" || result.logs[2].text != "● live line" {
		t.Fatalf("replayed history must come first: %#v", result.logs)
	}
}

func TestCommandMatchesOffersReverse(t *testing.T) {
	matches := commandMatches("/rev", 5)
	if len(matches) == 0 || matches[0] != "/reverse" {
		t.Fatalf("expected /reverse first, got %#v", matches)
	}
}

func TestCompactPreview(t *testing.T) {
	preview := compactPreview(strings.Repeat("x", 20)+"\nsecond\nthird", 2, 10)
	if !strings.Contains(preview, "xxxxxxxxxx…") || !strings.Contains(preview, "preview truncated") {
		t.Fatalf("unexpected preview %q", preview)
	}
}

func TestCompactPreviewKeepsMultiByteRunesIntact(t *testing.T) {
	preview := compactPreview(strings.Repeat("ы", 20), 4, 10)
	if !strings.HasPrefix(preview, strings.Repeat("ы", 10)+"…") {
		t.Fatalf("unexpected preview %q", preview)
	}
	if !utf8.ValidString(preview) {
		t.Fatalf("preview is not valid UTF-8: %q", preview)
	}
}

func TestCommandMatchesOffersBackgroundCommands(t *testing.T) {
	matches := commandMatches("/bg", 5)
	if len(matches) != 4 {
		t.Fatalf("expected four background commands, got %#v", matches)
	}
	for i, want := range []string{"/bg", "/bgs", "/bglog", "/bgstop"} {
		if matches[i] != want {
			t.Fatalf("expected %s at %d, got %#v", want, i, matches)
		}
	}
}

func TestStatusShowsBackgroundAgentCount(t *testing.T) {
	model := newChatModel("build", false, "test-model", "session", t.TempDir(), make(chan Input, 2), false)
	if status := model.statusView(); strings.Contains(status, "bg") {
		t.Fatalf("idle status should not mention background agents: %q", status)
	}
	updated, _ := model.Update(backgroundMsg{running: 3, limit: 25})
	status := updated.(chatModel).statusView()
	if !strings.Contains(status, "3/25 bg") {
		t.Fatalf("unexpected status %q", status)
	}
}

func TestChatModelEchoesUserInput(t *testing.T) {
	submitted := make(chan Input, 1)
	model := newChatModel("build", false, "test-model", "session", t.TempDir(), submitted, false)
	model.input.SetValue("fix the build")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	result := updated.(chatModel)
	if len(result.logs) != 1 || result.logs[0].kind != "user" || result.logs[0].text != "fix the build" {
		t.Fatalf("user message was not rendered: %#v", result.logs)
	}
	select {
	case input := <-submitted:
		if input.Text != "fix the build" {
			t.Fatalf("unexpected submitted text %q", input.Text)
		}
	default:
		t.Fatal("input was not submitted")
	}
}
