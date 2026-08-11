package ui

import (
	"strings"
	"testing"

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

func TestCompactPreview(t *testing.T) {
	preview := compactPreview(strings.Repeat("x", 20)+"\nsecond\nthird", 2, 10)
	if !strings.Contains(preview, "xxxxxxxxxx…") || !strings.Contains(preview, "preview truncated") {
		t.Fatalf("unexpected preview %q", preview)
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
