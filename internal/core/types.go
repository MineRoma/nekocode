package core

import (
	"encoding/json"
	"strings"
)

// Session modes. Build and Reverse expose the full tool surface; Plan is
// restricted to tools marked ReadOnly.
const (
	ModeBuild   = "build"
	ModePlan    = "plan"
	ModeReverse = "reverse"
)

// modeCycle is the order Tab walks through.
var modeCycle = []string{ModeBuild, ModePlan, ModeReverse}

// NormalizeMode maps any stored or user-typed value to a known mode. Unknown
// values fall back to Build so an old or hand-edited session still loads.
func NormalizeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ModePlan:
		return ModePlan
	case ModeReverse:
		return ModeReverse
	default:
		return ModeBuild
	}
}

// NextMode returns the mode Tab switches to.
func NextMode(mode string) string {
	current := NormalizeMode(mode)
	for i, candidate := range modeCycle {
		if candidate == current {
			return modeCycle[(i+1)%len(modeCycle)]
		}
	}
	return ModeBuild
}

// ModeLabel is the capitalized name shown in the status line.
func ModeLabel(mode string) string {
	switch NormalizeMode(mode) {
	case ModePlan:
		return "Plan"
	case ModeReverse:
		return "Reverse"
	default:
		return "Build"
	}
}

type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
	ReadOnly    bool           `json:"read_only"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func (u *Usage) Add(other Usage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
}

type PlanItem struct {
	Text   string `json:"text"`
	Status string `json:"status"`
}

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}
