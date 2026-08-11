package diff

import (
	"strings"
	"testing"
)

func TestUnified(t *testing.T) {
	got := Unified("main.go", "a\nb\n", "a\nc\n")
	for _, expected := range []string{"--- a/main.go", "+++ b/main.go", " a", "-b", "+c"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("diff missing %q:\n%s", expected, got)
		}
	}
}

func TestUnifiedNoChanges(t *testing.T) {
	if got := Unified("x", "same", "same"); got != "(no changes)" {
		t.Fatalf("unexpected result %q", got)
	}
}
