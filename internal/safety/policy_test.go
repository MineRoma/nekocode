package safety

import "testing"

func TestHardBlocks(t *testing.T) {
	commands := []string{
		"sudo apt update",
		"rm -rf /",
		"rm -fr /*",
		"rm -rf --no-preserve-root /tmp",
		"mkfs.ext4 /dev/sda",
		"shutdown now",
		":(){:|:&};:",
	}
	for _, command := range commands {
		policy := New(true, nil)
		err := policy.Authorize(Action{Kind: "command", Resource: command})
		if err == nil {
			t.Fatalf("expected %q to be blocked", command)
		}
	}
}

func TestYOLOAllowsNonBlockedActions(t *testing.T) {
	policy := New(true, nil)
	actions := []Action{
		{Kind: "write", Resource: ".env"},
		{Kind: "command", Resource: "git push origin main"},
		{Kind: "skill", Resource: "review"},
	}
	for _, action := range actions {
		if err := policy.Authorize(action); err != nil {
			t.Fatalf("unexpected denial for %#v: %v", action, err)
		}
	}
}

func TestAllowForSession(t *testing.T) {
	prompts := 0
	policy := New(false, func(Action) Decision {
		prompts++
		return AllowSession
	})
	action := Action{Kind: "command", Resource: "go test ./..."}
	if err := policy.Authorize(action); err != nil {
		t.Fatal(err)
	}
	if err := policy.Authorize(action); err != nil {
		t.Fatal(err)
	}
	if prompts != 1 {
		t.Fatalf("expected one prompt, got %d", prompts)
	}
}

// Background agents are built with a nil prompt. Ask mode must deny instead of
// blocking, because a detached agent has nobody to answer it.
func TestNilPromptDeniesWithoutBlocking(t *testing.T) {
	policy := New(false, nil)
	err := policy.Authorize(Action{Kind: "write", Resource: "main.go"})
	if err == nil {
		t.Fatal("expected a non-interactive policy to deny writes")
	}
	if got := err.Error(); got != "permission denied: no interactive prompt available" {
		t.Fatalf("unexpected error %q", got)
	}
	// The same policy in YOLO mode proceeds without a prompt.
	if err := New(true, nil).Authorize(Action{Kind: "write", Resource: "main.go"}); err != nil {
		t.Fatalf("unexpected denial in YOLO mode: %v", err)
	}
}
