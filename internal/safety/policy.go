package safety

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type Decision int

const (
	Deny Decision = iota
	AllowOnce
	AllowSession
)

type Action struct {
	Kind        string
	Resource    string
	Description string
	Preview     string
}

type Prompt func(Action) Decision

type Policy struct {
	YOLO    bool
	Prompt  Prompt
	allowed map[string]bool
}

func New(yolo bool, prompt Prompt) *Policy {
	return &Policy{YOLO: yolo, Prompt: prompt, allowed: map[string]bool{}}
}

func (p *Policy) Authorize(action Action) error {
	if reason := hardBlock(action); reason != "" {
		return fmt.Errorf("blocked by safety policy: %s", reason)
	}
	if p.YOLO {
		return nil
	}
	key := action.Kind + ":" + action.Resource
	if p.allowed[key] || p.allowed[action.Kind+":*"] {
		return nil
	}
	if p.Prompt == nil {
		return errors.New("permission denied: no interactive prompt available")
	}
	switch p.Prompt(action) {
	case AllowOnce:
		return nil
	case AllowSession:
		p.allowed[key] = true
		return nil
	default:
		return errors.New("permission denied by user")
	}
}

func hardBlock(action Action) string {
	if action.Kind != "command" {
		return ""
	}
	cmd := strings.TrimSpace(strings.ToLower(action.Resource))
	if regexp.MustCompile(`(^|[;&|]\s*|\s)sudo(\s|$)`).MatchString(cmd) {
		return "sudo is never allowed"
	}
	if regexp.MustCompile(`\brm\s+(-[a-z]*r[a-z]*f[a-z]*|-[a-z]*f[a-z]*r[a-z]*)\s+(/|/\*|--no-preserve-root)(\s|$)`).MatchString(cmd) {
		return "destructive deletion of the filesystem root is never allowed"
	}
	if strings.Contains(cmd, "--no-preserve-root") {
		return "--no-preserve-root is never allowed"
	}
	if regexp.MustCompile(`\b(mkfs(\.|\s)|shutdown\s|reboot\s|poweroff\s)`).MatchString(cmd) {
		return "machine-wide destructive commands are never allowed"
	}
	if strings.Contains(cmd, ":(){:|:&};:") {
		return "fork bombs are never allowed"
	}
	return ""
}
