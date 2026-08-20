package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/m1neroma/neko/internal/config"
	"github.com/m1neroma/neko/internal/core"
	ndiff "github.com/m1neroma/neko/internal/diff"
	"github.com/m1neroma/neko/internal/project"
	"github.com/m1neroma/neko/internal/safety"
	"github.com/m1neroma/neko/internal/session"
	"github.com/m1neroma/neko/internal/skills"
)

type Reporter interface {
	Tool(name, detail string)
	Ask(question string, options []core.QuestionOption) (string, error)
}

type Registry struct {
	root     string
	policy   *safety.Policy
	session  *session.Session
	save     func(*session.Session) error
	skills   *skills.Store
	reporter Reporter
}

func New(root string, policy *safety.Policy, current *session.Session, save func(*session.Session) error, skillStore *skills.Store, reporter Reporter) *Registry {
	return &Registry{root: root, policy: policy, session: current, save: save, skills: skillStore, reporter: reporter}
}

// SkillsForMode lists the enabled skills usable in the given mode.
func (r *Registry) SkillsForMode(mode string) []config.Skill {
	if r.skills == nil {
		return nil
	}
	return r.skills.ForMode(mode)
}

func (r *Registry) Definitions(mode string) []core.ToolDefinition {
	all := []core.ToolDefinition{
		{Name: "read_file", Description: "Read a UTF-8 text file inside the project with line numbers.", ReadOnly: true, InputSchema: object(map[string]any{"path": stringProp("Project-relative file path"), "offset": integerProp("First line, starting at 1"), "limit": integerProp("Maximum lines")}, "path")},
		{Name: "list_files", Description: "List project files. Use an optional relative directory.", ReadOnly: true, InputSchema: object(map[string]any{"path": stringProp("Optional project-relative directory"), "limit": integerProp("Maximum entries")})},
		{Name: "search", Description: "Search text in project files. Returns matching lines and locations.", ReadOnly: true, InputSchema: object(map[string]any{"query": stringProp("Literal or regular expression"), "path": stringProp("Optional relative path"), "glob": stringProp("Optional file glob such as *.go")}, "query")},
		{Name: "git_diff", Description: "Show the current git diff, optionally for one project-relative path.", ReadOnly: true, InputSchema: object(map[string]any{"path": stringProp("Optional project-relative path")})},
		{Name: "write_file", Description: "Create or fully replace a project file. A reviewable diff is shown before approval in Ask mode.", InputSchema: object(map[string]any{"path": stringProp("Project-relative file path"), "content": stringProp("Complete new file content")}, "path", "content")},
		{Name: "replace_in_file", Description: "Replace one exact, unique string in a project file. Prefer this for targeted edits.", InputSchema: object(map[string]any{"path": stringProp("Project-relative file path"), "old": stringProp("Exact text to replace"), "new": stringProp("Replacement text")}, "path", "old", "new")},
		{Name: "run_command", Description: "Run a shell command in the project root. Commands require approval in Ask mode and are audited.", InputSchema: object(map[string]any{"command": stringProp("Shell command"), "reason": stringProp("Why the command is needed")}, "command")},
		{Name: "run_tests", Description: "Detect and run the project's standard test command.", InputSchema: object(map[string]any{"command": stringProp("Optional explicit test command")})},
		{Name: "update_plan", Description: "Create or update the visible task plan. Status must be pending, in_progress, done, or failed.", ReadOnly: true, InputSchema: object(map[string]any{"items": map[string]any{"type": "array", "items": map[string]any{"type": "object", "properties": map[string]any{"text": stringProp("Plan step"), "status": stringProp("Step status")}, "required": []string{"text", "status"}}}}, "items")},
		{Name: "ask_user", Description: "Ask one material clarification question. Supply two or three concise choices; Neko always adds a custom-answer choice.", ReadOnly: true, InputSchema: object(map[string]any{"question": stringProp("Clear question for the user"), "options": map[string]any{"type": "array", "minItems": 3, "maxItems": 3, "items": map[string]any{"type": "object", "properties": map[string]any{"label": stringProp("Short option label"), "description": stringProp("Optional one-line consequence")}, "required": []string{"label"}, "additionalProperties": false}}}, "question", "options")},
		{Name: "load_skill", Description: "Load the instructions from a configured skill.", ReadOnly: true, InputSchema: object(map[string]any{"name": stringProp("Configured skill name")}, "name")},
		{Name: "add_skill", Description: "Register a local skill directory containing SKILL.md. Requires permission in Ask mode.", InputSchema: object(map[string]any{"name": stringProp("Skill name"), "path": stringProp("Local skill directory")}, "name", "path")},
	}
	// Build and Reverse expose every tool; only Plan is restricted to read-only.
	if core.NormalizeMode(mode) != core.ModePlan {
		return all
	}
	var readonly []core.ToolDefinition
	for _, tool := range all {
		if tool.ReadOnly {
			readonly = append(readonly, tool)
		}
	}
	return readonly
}

func (r *Registry) Execute(ctx context.Context, call core.ToolCall) (string, error) {
	var args map[string]json.RawMessage
	if err := json.Unmarshal(call.Arguments, &args); err != nil {
		return "", fmt.Errorf("invalid tool arguments: %w", err)
	}
	r.reporter.Tool(call.Name, detail(call.Name, args))
	switch call.Name {
	case "read_file":
		path, err := r.path(argString(args, "path"))
		if err != nil {
			return "", err
		}
		return project.ReadLines(path, argInt(args, "offset", 1), argInt(args, "limit", 400))
	case "list_files":
		return r.listFiles(argString(args, "path"), argInt(args, "limit", 500))
	case "search":
		return r.search(ctx, argString(args, "query"), argString(args, "path"), argString(args, "glob"))
	case "git_diff":
		return r.gitDiff(ctx, argString(args, "path"))
	case "write_file":
		return r.writeFile(argString(args, "path"), argString(args, "content"))
	case "replace_in_file":
		return r.replaceInFile(argString(args, "path"), argString(args, "old"), argString(args, "new"))
	case "run_command":
		return r.runCommand(ctx, argString(args, "command"), argString(args, "reason"))
	case "run_tests":
		cmd := argString(args, "command")
		if cmd == "" {
			cmd = r.detectTests()
		}
		if cmd == "" {
			return "", errors.New("could not detect a test command")
		}
		return r.runCommand(ctx, cmd, "Run project tests")
	case "update_plan":
		return r.updatePlan(args["items"])
	case "ask_user":
		return r.askUser(argString(args, "question"), args["options"])
	case "load_skill":
		return r.skills.Load(argString(args, "name"))
	case "add_skill":
		return r.addSkill(argString(args, "name"), argString(args, "path"))
	default:
		return "", fmt.Errorf("unknown tool %q", call.Name)
	}
}

func (r *Registry) writeFile(requested, content string) (string, error) {
	path, err := r.path(requested)
	if err != nil {
		return "", err
	}
	before, readErr := os.ReadFile(path)
	beforeExists := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return "", readErr
	}
	rel, _ := filepath.Rel(r.root, path)
	preview := ndiff.Unified(filepath.ToSlash(rel), string(before), content)
	description := "Write " + filepath.ToSlash(rel)
	if isEnvFile(rel) {
		description += " (sensitive environment file)"
	}
	if err := r.policy.Authorize(safety.Action{Kind: "write", Resource: filepath.ToSlash(rel), Description: description, Preview: preview}); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp := path + ".neko-tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	r.recordChange(rel, string(before), content, beforeExists)
	return preview, nil
}

func (r *Registry) replaceInFile(requested, old, replacement string) (string, error) {
	if old == "" {
		return "", errors.New("old text cannot be empty")
	}
	path, err := r.path(requested)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	count := strings.Count(string(data), old)
	if count != 1 {
		return "", fmt.Errorf("old text must match exactly once; found %d matches", count)
	}
	after := strings.Replace(string(data), old, replacement, 1)
	return r.writeFile(requested, after)
}

func (r *Registry) runCommand(ctx context.Context, command, reason string) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", errors.New("command is required")
	}
	description := reason
	if description == "" {
		description = "Run shell command"
	}
	if strings.Contains(strings.ToLower(command), "git push") {
		description += " (publishes changes to a remote)"
	}
	if err := r.policy.Authorize(safety.Action{Kind: "command", Resource: command, Description: description, Preview: "$ " + command}); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "sh", "-lc", command)
	cmd.Dir = r.root
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	text := string(output)
	if len(text) > 200_000 {
		text = text[len(text)-200_000:]
		text = "... output truncated to final 200 KB ...\n" + text
	}
	if err != nil {
		return text, fmt.Errorf("command failed: %w", err)
	}
	if text == "" {
		text = "Command completed successfully with no output."
	}
	return text, nil
}

func (r *Registry) search(ctx context.Context, query, requested, glob string) (string, error) {
	if query == "" {
		return "", errors.New("query is required")
	}
	path := r.root
	var err error
	if requested != "" {
		path, err = r.path(requested)
		if err != nil {
			return "", err
		}
	}
	args := []string{"--line-number", "--column", "--hidden", "--glob", "!.git/**", "--glob", "!node_modules/**", "--max-count", "200"}
	if glob != "" {
		args = append(args, "--glob", glob)
	}
	args = append(args, query, path)
	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = r.root
	output, err := cmd.CombinedOutput()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
			return "No matches found.", nil
		}
		return string(output), err
	}
	if len(output) > 150_000 {
		output = append(output[:150_000], []byte("\n... results truncated ...")...)
	}
	return string(output), nil
}

func (r *Registry) gitDiff(ctx context.Context, requested string) (string, error) {
	args := []string{"-C", r.root, "diff", "--"}
	if requested != "" {
		path, err := r.path(requested)
		if err != nil {
			return "", err
		}
		args = append(args, path)
	}
	output, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	if err != nil {
		return string(output), err
	}
	if len(output) == 0 {
		return "No unstaged changes.", nil
	}
	return string(output), nil
}

func (r *Registry) listFiles(requested string, limit int) (string, error) {
	path := r.root
	var err error
	if requested != "" {
		path, err = r.path(requested)
		if err != nil {
			return "", err
		}
	}
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	var paths []string
	ignored := map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "target": true}
	err = filepath.WalkDir(path, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() && ignored[d.Name()] && current != path {
			return filepath.SkipDir
		}
		if current == path {
			return nil
		}
		if len(paths) >= limit {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(r.root, current)
		if err == nil {
			if d.IsDir() {
				rel += "/"
			}
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(paths)
	if len(paths) == limit {
		paths = append(paths, "... truncated ...")
	}
	return strings.Join(paths, "\n"), err
}

func (r *Registry) updatePlan(raw json.RawMessage) (string, error) {
	var items []core.PlanItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return "", err
	}
	for _, item := range items {
		switch item.Status {
		case "pending", "in_progress", "done", "failed":
		default:
			return "", fmt.Errorf("invalid plan status %q", item.Status)
		}
		if strings.TrimSpace(item.Text) == "" {
			return "", errors.New("plan item text cannot be empty")
		}
	}
	r.session.Plan = items
	if err := r.save(r.session); err != nil {
		return "", err
	}
	var out strings.Builder
	for _, item := range items {
		fmt.Fprintf(&out, "[%s] %s\n", item.Status, item.Text)
	}
	return out.String(), nil
}

func (r *Registry) askUser(question string, raw json.RawMessage) (string, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return "", errors.New("question is required")
	}
	var options []core.QuestionOption
	if err := json.Unmarshal(raw, &options); err != nil {
		return "", err
	}
	if len(options) != 3 {
		return "", errors.New("ask_user requires exactly three options")
	}
	for i := range options {
		options[i].Label = strings.TrimSpace(options[i].Label)
		options[i].Description = strings.TrimSpace(options[i].Description)
		if options[i].Label == "" {
			return "", errors.New("option labels cannot be empty")
		}
	}
	return r.reporter.Ask(question, options)
}

func (r *Registry) addSkill(name, path string) (string, error) {
	if err := r.policy.Authorize(safety.Action{Kind: "skill", Resource: name, Description: "Register skill " + name, Preview: path}); err != nil {
		return "", err
	}
	if err := r.skills.Add(name, path); err != nil {
		return "", err
	}
	return "Skill " + name + " registered.", nil
}

func (r *Registry) CreateCheckpoint(label string) (string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "manual"
	}
	checkpoint := session.Checkpoint{
		ID: fmt.Sprintf("cp-%d", time.Now().UnixNano()), Label: label,
		ChangeIndex: len(r.session.Changes), MessageIndex: len(r.session.Messages), CreatedAt: time.Now(),
	}
	r.session.Checkpoints = append(r.session.Checkpoints, checkpoint)
	if err := r.save(r.session); err != nil {
		return "", err
	}
	return checkpoint.Label, nil
}

func (r *Registry) RestoreLatest() (string, error) {
	if len(r.session.Checkpoints) == 0 {
		return "", errors.New("no checkpoint available")
	}
	checkpoint := r.session.Checkpoints[len(r.session.Checkpoints)-1]
	if checkpoint.ChangeIndex < 0 || checkpoint.ChangeIndex > len(r.session.Changes) {
		return "", errors.New("checkpoint change index is invalid")
	}
	for i := len(r.session.Changes) - 1; i >= checkpoint.ChangeIndex; i-- {
		change := r.session.Changes[i]
		path, err := r.path(change.Path)
		if err != nil {
			return "", err
		}
		if change.BeforeExists {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(path, []byte(change.Before), 0o644); err != nil {
				return "", err
			}
		} else if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	r.session.Changes = r.session.Changes[:checkpoint.ChangeIndex]
	if checkpoint.MessageIndex >= 0 && checkpoint.MessageIndex <= len(r.session.Messages) {
		r.session.Messages = r.session.Messages[:checkpoint.MessageIndex]
	}
	r.session.Checkpoints = r.session.Checkpoints[:len(r.session.Checkpoints)-1]
	if err := r.save(r.session); err != nil {
		return "", err
	}
	return checkpoint.Label, nil
}

func (r *Registry) Checkpoints() []session.Checkpoint {
	return append([]session.Checkpoint(nil), r.session.Checkpoints...)
}

func (r *Registry) Undo() (string, error) {
	if len(r.session.Changes) == 0 {
		return "", errors.New("nothing to undo")
	}
	change := r.session.Changes[len(r.session.Changes)-1]
	path, err := r.path(change.Path)
	if err != nil {
		return "", err
	}
	if change.BeforeExists {
		if err := os.WriteFile(path, []byte(change.Before), 0o644); err != nil {
			return "", err
		}
	} else if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	r.session.Changes = r.session.Changes[:len(r.session.Changes)-1]
	if err := r.save(r.session); err != nil {
		return "", err
	}
	return "Restored " + filepath.ToSlash(change.Path), nil
}

func (r *Registry) recordChange(path, before, after string, beforeExists bool) {
	r.session.Changes = append(r.session.Changes, session.Change{Path: path, Before: before, After: after, BeforeExists: beforeExists, At: time.Now()})
	_ = r.save(r.session)
}

func (r *Registry) detectTests() string {
	candidates := []struct{ file, command string }{
		{"go.mod", "go test ./..."}, {"package.json", "npm test -- --runInBand"}, {"pyproject.toml", "pytest"},
		{"pytest.ini", "pytest"}, {"Cargo.toml", "cargo test"}, {"pom.xml", "mvn test"}, {"build.gradle", "./gradlew test"},
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(r.root, candidate.file)); err == nil {
			return candidate.command
		}
	}
	return ""
}

func (r *Registry) path(requested string) (string, error) {
	return project.ResolveInside(r.root, requested)
}

func isEnvFile(path string) bool {
	name := filepath.Base(path)
	return name == ".env" || strings.HasPrefix(name, ".env.")
}

func detail(name string, args map[string]json.RawMessage) string {
	for _, key := range []string{"path", "command", "query", "name"} {
		if value := argString(args, key); value != "" {
			if len(value) > 90 {
				value = value[:90] + "…"
			}
			return value
		}
	}
	return ""
}

func argString(args map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(args[key], &value)
	return value
}

func argInt(args map[string]json.RawMessage, key string, fallback int) int {
	var value int
	if err := json.Unmarshal(args[key], &value); err != nil || value == 0 {
		return fallback
	}
	return value
}

func object(properties map[string]any, required ...string) map[string]any {
	result := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		result["required"] = required
	}
	return result
}

func stringProp(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integerProp(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}
