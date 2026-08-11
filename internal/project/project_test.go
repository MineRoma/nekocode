package project

import (
	"path/filepath"
	"testing"
)

func TestResolveInside(t *testing.T) {
	root := t.TempDir()
	path, err := ResolveInside(root, "internal/app.go")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "internal", "app.go")
	if path != want {
		t.Fatalf("got %q, want %q", path, want)
	}
}

func TestResolveInsideRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveInside(root, "../secret"); err == nil {
		t.Fatal("expected project escape to fail")
	}
}
