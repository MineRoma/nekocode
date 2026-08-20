package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/m1neroma/neko/internal/agent"
	"github.com/m1neroma/neko/internal/background"
	"github.com/m1neroma/neko/internal/config"
	"github.com/m1neroma/neko/internal/core"
	"github.com/m1neroma/neko/internal/project"
	"github.com/m1neroma/neko/internal/provider"
	"github.com/m1neroma/neko/internal/safety"
	"github.com/m1neroma/neko/internal/session"
	"github.com/m1neroma/neko/internal/skills"
	"github.com/m1neroma/neko/internal/tools"
	"github.com/m1neroma/neko/internal/ui"
)

type Options struct {
	Continue       bool
	Resume         string
	YOLO           bool
	BackgroundJobs int
}

type App struct {
	options     Options
	ui          *ui.UI
	config      *config.Store
	project     project.Context
	manager     *session.Manager
	session     *session.Session
	policy      *safety.Policy
	skills      *skills.Store
	skillRoot   string
	tools       *tools.Registry
	background  *background.Manager
	lastRefresh time.Time
}

func Run(ctx context.Context, options Options) error {
	if options.Continue && options.Resume != "" {
		return errors.New("--continue and --resume cannot be used together")
	}
	projectContext, err := project.Discover()
	if err != nil {
		return err
	}
	cfg, err := config.Open()
	if err != nil {
		return err
	}
	manager, err := session.NewManager(projectContext.Root)
	if err != nil {
		return err
	}
	current, err := loadSession(manager, projectContext.Root, options)
	if err != nil {
		return err
	}
	terminal := ui.New()
	policy := safety.New(options.YOLO, terminal.Permission)
	skillStore := skills.New(cfg)
	registry := tools.New(projectContext.Root, policy, current, manager.Save, skillStore, terminal)
	a := &App{
		options: options, ui: terminal, config: cfg, project: projectContext,
		manager: manager, session: current, policy: policy, skills: skillStore, tools: registry,
		skillRoot: bundledSkillRoot(),
	}
	a.background = background.New(options.BackgroundJobs, a.newBackgroundAgent, a.onBackgroundEvent)
	if err := a.ensureProvider(ctx); err != nil {
		return err
	}
	a.refreshStaleModels(ctx, true)
	// A session resumed in a mode with bundled skills gets them registered too.
	a.installBundledSkills(a.session.Mode)
	a.ui.Header(a.session.Mode, options.YOLO, a.config.Get().ActiveModel, a.session.ID)
	// A resumed session shows its transcript; a fresh one has nothing to replay.
	if options.Continue || options.Resume != "" {
		a.ui.Replay(a.session.Messages, a.session.Summary)
	}
	defer a.ui.Close()
	defer a.background.Shutdown(5 * time.Second)
	for {
		a.refreshStaleModels(ctx, false)
		input, err := a.ui.ReadInput(a.session.Mode)
		if errors.Is(err, ui.ErrInterrupted) || errors.Is(err, os.ErrClosed) {
			a.ui.Println("Bye.")
			return nil
		}
		if err != nil && input.Text == "" {
			return err
		}
		if input.ToggleMode {
			a.toggleMode()
			continue
		}
		if input.Text == "" {
			continue
		}
		handled, exit, err := a.command(ctx, input.Text)
		if err != nil {
			a.ui.Error(err)
			continue
		}
		if exit {
			return nil
		}
		if handled {
			continue
		}
		codingAgent, err := a.agent()
		if err != nil {
			a.ui.Error(err)
			continue
		}
		checkpoint, err := a.tools.CreateCheckpoint(checkpointLabel(input.Text))
		if err != nil {
			a.ui.Error(err)
			continue
		}
		a.ui.Info("Checkpoint created: " + checkpoint + " · use /restore to roll back this turn")
		if err := codingAgent.Run(ctx, input.Text); err != nil {
			a.ui.Error(err)
		}
	}
}

// bundledSkillRoot is where Neko unpacks the skills embedded in the binary.
// It sits in the user data directory, not the project, so switching modes never
// writes into a repository. An empty result disables bundled skills.
func bundledSkillRoot() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "neko", "skills")
}

func loadSession(manager *session.Manager, root string, options Options) (*session.Session, error) {
	if options.Resume != "" {
		current, err := manager.Load(options.Resume)
		if err != nil {
			return nil, fmt.Errorf("resume session %s: %w", options.Resume, err)
		}
		return current, nil
	}
	if options.Continue {
		current, err := manager.Latest()
		if err == nil {
			return current, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return manager.New(root)
}

func (a *App) agent() (*agent.Agent, error) {
	cfg := a.config.Get()
	providerConfig, ok := cfg.ActiveProviderConfig()
	if !ok {
		return nil, errors.New("no active provider; use /addprovider")
	}
	if cfg.ActiveModel == "" {
		return nil, errors.New("no active model; use /model")
	}
	client, err := provider.New(providerConfig)
	if err != nil {
		return nil, err
	}
	return agent.New(client, a.config, a.project, a.session, a.manager.Save, a.tools, a.ui), nil
}

func (a *App) command(ctx context.Context, input string) (bool, bool, error) {
	if !strings.HasPrefix(input, "/") {
		return false, false, nil
	}
	parts := strings.Fields(input)
	command := strings.ToLower(parts[0])
	args := parts[1:]
	switch command {
	case "/exit", "/quit":
		a.ui.Println("Bye.")
		return true, true, nil
	case "/help":
		a.ui.Println(helpText)
	case "/build":
		a.setMode(core.ModeBuild)
	case "/plan":
		a.setMode(core.ModePlan)
	case "/reverse":
		a.setMode(core.ModeReverse)
	case "/mode":
		a.toggleMode()
	case "/addprovider", "/addprovide":
		return true, false, a.addProvider(ctx)
	case "/providers":
		return true, false, a.selectProvider()
	case "/model", "/models":
		return true, false, a.selectModel()
	case "/compact":
		codingAgent, err := a.agent()
		if err != nil {
			return true, false, err
		}
		return true, false, codingAgent.Compact(ctx)
	case "/autocompact":
		return true, false, a.autoCompact()
	case "/context":
		codingAgent, err := a.agent()
		if err != nil {
			return true, false, err
		}
		tokens, window, ratio := codingAgent.ContextStats()
		a.ui.Info(fmt.Sprintf("Context: ~%d / %d tokens (%.1f%%)", tokens, window, ratio*100))
	case "/cost":
		cfg := a.config.Get()
		usage := a.session.Usage
		cost := cfg.EstimatedCost(usage.InputTokens, usage.OutputTokens)
		if cost > 0 {
			a.ui.Info(fmt.Sprintf("Usage: %d input + %d output tokens · estimated $%.4f", usage.InputTokens, usage.OutputTokens, cost))
		} else {
			a.ui.Info(fmt.Sprintf("Usage: %d input + %d output tokens · pricing not configured", usage.InputTokens, usage.OutputTokens))
		}
	case "/diff":
		result, err := a.readTool(ctx, "git_diff", map[string]any{})
		if err != nil {
			return true, false, err
		}
		a.ui.Println(result)
	case "/undo":
		result, err := a.tools.Undo()
		if err != nil {
			return true, false, err
		}
		a.ui.Success(result)
	case "/checkpoint":
		label := strings.Join(args, " ")
		result, err := a.tools.CreateCheckpoint(label)
		if err != nil {
			return true, false, err
		}
		a.ui.Success("Checkpoint created: " + result)
	case "/restore":
		result, err := a.tools.RestoreLatest()
		if err != nil {
			return true, false, err
		}
		a.ui.Success("Restored checkpoint: " + result)
	case "/checkpoints":
		items := a.tools.Checkpoints()
		if len(items) == 0 {
			a.ui.Println("No checkpoints available.")
		} else {
			for _, item := range items {
				a.ui.Println("- " + item.Label + "  " + item.CreatedAt.Format(time.RFC3339))
			}
		}
	case "/skills":
		a.printSkills()
	case "/addskill":
		return true, false, a.addSkill(args)
	case "/addskills":
		return true, false, a.addSkillSet(args)
	case "/permissions":
		mode := "ASK"
		if a.options.YOLO {
			mode = "YOLO"
		}
		a.ui.Println("Permission mode: " + mode + "\nAlways blocked: sudo, rm -rf /, --no-preserve-root, mkfs, shutdown, reboot, and fork bombs.")
	case "/session":
		a.ui.Info("Session: " + a.session.ID)
	case "/sessions":
		return true, false, a.printSessions()
	case "/bg", "/background":
		return true, false, a.spawnBackground(ctx, commandArgument(input, parts[0]))
	case "/bgs", "/jobs":
		a.printBackground()
	case "/bglog", "/bgoutput":
		return true, false, a.printBackgroundLog(args)
	case "/bgstop", "/bgcancel":
		return true, false, a.cancelBackground(args)
	default:
		return true, false, fmt.Errorf("unknown command %s; use /help", command)
	}
	return true, false, nil
}

// toggleMode advances Build → Plan → Reverse → Build, matching the Tab key.
func (a *App) toggleMode() {
	a.setMode(core.NextMode(a.session.Mode))
}

func (a *App) setMode(mode string) {
	a.session.Mode = core.NormalizeMode(mode)
	_ = a.manager.Save(a.session)
	a.installBundledSkills(a.session.Mode)
	a.ui.Header(a.session.Mode, a.options.YOLO, a.config.Get().ActiveModel, a.session.ID)
}

// installBundledSkills registers the skills shipped for a mode the first time
// that mode is entered, so Reverse mode works without a manual /addskills.
// Bundled skills are Neko's own content written to its own data directory, so
// this does not go through the permission policy — no project file is touched.
func (a *App) installBundledSkills(mode string) {
	if a.skillRoot == "" {
		return
	}
	added, err := a.skills.InstallBundled(a.skillRoot, mode)
	if err != nil {
		a.ui.Warn("Could not install bundled " + core.ModeLabel(mode) + " skills: " + err.Error())
		return
	}
	if len(added) == 0 {
		return
	}
	a.ui.Success(fmt.Sprintf("Registered %d bundled %s skills · /skills lists them", len(added), core.ModeLabel(mode)))
}

func (a *App) ensureProvider(ctx context.Context) error {
	cfg := a.config.Get()
	if len(cfg.Providers) == 0 || cfg.ActiveProvider == "" {
		a.ui.Info("No provider configured. Let's add one.")
		return a.addProvider(ctx)
	}
	if _, ok := cfg.ActiveProviderConfig(); !ok {
		return errors.New("active provider does not exist; use /addprovider")
	}
	return nil
}

func (a *App) addProvider(ctx context.Context) error {
	choice, err := a.ui.Select("Provider compatibility", []string{"OpenAI-compatible", "Anthropic"}, 0)
	if err != nil {
		return err
	}
	compatibility := "openai"
	defaultName := "openai"
	defaultBase := "https://api.openai.com/v1"
	defaultEnv := "OPENAI_API_KEY"
	if choice == 1 {
		compatibility = "anthropic"
		defaultName = "anthropic"
		defaultBase = "https://api.anthropic.com/v1"
		defaultEnv = "ANTHROPIC_API_KEY"
	}
	name, err := a.ui.Prompt("Name [" + defaultName + "]:")
	if err != nil {
		return err
	}
	if name == "" {
		name = defaultName
	}
	base, err := a.ui.Prompt("Base URL [" + defaultBase + "]:")
	if err != nil {
		return err
	}
	if base == "" {
		base = defaultBase
	}
	authChoice, err := a.ui.Select("Authentication", []string{"Paste API key (saved locally)", "Environment variable"}, 0)
	if err != nil {
		return err
	}
	var apiKey, env string
	if authChoice == 0 {
		apiKey, err = a.ui.PromptSecret("API key")
		if err != nil {
			return err
		}
		if apiKey == "" {
			return errors.New("an API key is required")
		}
	} else {
		env, err = a.ui.Prompt("Environment variable [" + defaultEnv + "]")
		if err != nil {
			return err
		}
		if env == "" {
			env = defaultEnv
		}
	}
	entry := config.Provider{Name: name, Compatibility: compatibility, BaseURL: strings.TrimRight(base, "/"), APIKey: apiKey, APIKeyEnv: env}
	if err := a.config.Update(func(cfg *config.Config) error {
		replaced := false
		for i := range cfg.Providers {
			if cfg.Providers[i].Name == name {
				cfg.Providers[i] = entry
				replaced = true
				break
			}
		}
		if !replaced {
			cfg.Providers = append(cfg.Providers, entry)
		}
		cfg.ActiveProvider = name
		cfg.ActiveModel = ""
		return nil
	}); err != nil {
		return err
	}
	if err := a.refreshProviderModels(ctx, name); err != nil {
		a.ui.Warn("Could not import models: " + err.Error())
		model, promptErr := a.ui.Prompt("Model ID:")
		if promptErr != nil {
			return promptErr
		}
		if model == "" {
			return errors.New("a model ID is required")
		}
		return a.config.Update(func(cfg *config.Config) error { cfg.ActiveModel = model; return nil })
	}
	return a.selectModel()
}

func (a *App) selectProvider() error {
	cfg := a.config.Get()
	if len(cfg.Providers) == 0 {
		return errors.New("no providers configured")
	}
	options := make([]string, len(cfg.Providers))
	selected := 0
	for i, item := range cfg.Providers {
		options[i] = item.Name + "  (" + item.Compatibility + ")"
		if item.Name == cfg.ActiveProvider {
			selected = i
		}
	}
	choice, err := a.ui.Select("Select provider", options, selected)
	if err != nil {
		return err
	}
	providerName := cfg.Providers[choice].Name
	return a.config.Update(func(current *config.Config) error {
		current.ActiveProvider = providerName
		current.ActiveModel = ""
		for _, item := range current.Providers {
			if item.Name == providerName && len(item.Models) > 0 {
				current.ActiveModel = item.Models[0].ID
			}
		}
		return nil
	})
}

func (a *App) selectModel() error {
	cfg := a.config.Get()
	providerConfig, ok := cfg.ActiveProviderConfig()
	if !ok {
		return errors.New("no active provider")
	}
	if len(providerConfig.Models) == 0 {
		model, err := a.ui.Prompt("Model ID:")
		if err != nil {
			return err
		}
		if model == "" {
			return errors.New("model ID is required")
		}
		return a.config.Update(func(current *config.Config) error { current.ActiveModel = model; return nil })
	}
	options := make([]string, len(providerConfig.Models))
	selected := 0
	for i, model := range providerConfig.Models {
		options[i] = model.ID
		if model.ID == cfg.ActiveModel {
			selected = i
		}
	}
	choice, err := a.ui.Select("Select model", options, selected)
	if err != nil {
		return err
	}
	return a.config.Update(func(current *config.Config) error { current.ActiveModel = options[choice]; return nil })
}

func (a *App) autoCompact() error {
	cfg := a.config.Get()
	selected := 0
	if !cfg.AutoCompact {
		selected = 1
	}
	choice, err := a.ui.Select("Auto-compact at 90% context", []string{"Enabled", "Disabled"}, selected)
	if err != nil {
		return err
	}
	enabled := choice == 0
	if err := a.config.Update(func(current *config.Config) error { current.AutoCompact = enabled; return nil }); err != nil {
		return err
	}
	a.ui.Success(fmt.Sprintf("Auto-compact %s.", map[bool]string{true: "enabled", false: "disabled"}[enabled]))
	return nil
}

func (a *App) addSkill(args []string) error {
	var name, path string
	if len(args) >= 2 {
		name, path = args[0], strings.Join(args[1:], " ")
	} else {
		var err error
		name, err = a.ui.Prompt("Skill name:")
		if err != nil {
			return err
		}
		path, err = a.ui.Prompt("Skill directory:")
		if err != nil {
			return err
		}
	}
	if err := a.policy.Authorize(safety.Action{Kind: "skill", Resource: name, Description: "Register skill " + name, Preview: path}); err != nil {
		return err
	}
	if err := a.skills.Add(name, path); err != nil {
		return err
	}
	a.ui.Success("Skill registered: " + name)
	return nil
}

// addSkillSet registers every skill directory under one parent. With no
// argument it reinstalls Neko's bundled skills for the current mode, which is
// how a user recovers a skill they deleted.
func (a *App) addSkillSet(args []string) error {
	dir := strings.Join(args, " ")
	if strings.TrimSpace(dir) == "" {
		added, err := a.skills.InstallBundled(a.skillRoot, a.session.Mode)
		if err != nil {
			return err
		}
		label := core.ModeLabel(a.session.Mode)
		if len(added) == 0 {
			a.ui.Info("Bundled " + label + " skills are already registered. Pass a directory to add your own.")
			return nil
		}
		a.ui.Success(fmt.Sprintf("Registered %d bundled %s skills", len(added), label))
		return nil
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(a.project.Root, dir)
	}
	found, err := skills.Discover(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(found))
	for _, descriptor := range found {
		names = append(names, descriptor.Name)
	}
	preview := dir + "\n\n" + strings.Join(names, "\n")
	if err := a.policy.Authorize(safety.Action{
		Kind: "skill", Resource: dir,
		Description: fmt.Sprintf("Register %d skills from %s", len(found), dir),
		Preview:     preview,
	}); err != nil {
		return err
	}
	registered := 0
	for _, descriptor := range found {
		if err := a.skills.Add(descriptor.Name, descriptor.Path); err != nil {
			a.ui.Warn("Skipped " + descriptor.Name + ": " + err.Error())
			continue
		}
		registered++
	}
	a.ui.Success(fmt.Sprintf("Registered %d of %d skills from %s", registered, len(found), dir))
	return nil
}

func (a *App) printSkills() {
	items := a.skills.List()
	if len(items) == 0 {
		a.ui.Println("No skills configured. Use /addskill or /addskills.")
		return
	}
	current := core.NormalizeMode(a.session.Mode)
	for _, skill := range items {
		status := "disabled"
		if skill.Enabled {
			status = "enabled"
		}
		scope := "all modes"
		if skill.Mode != "" {
			scope = core.ModeLabel(skill.Mode) + " mode"
			if core.NormalizeMode(skill.Mode) != current {
				scope += ", inactive now"
			}
		}
		line := fmt.Sprintf("- %s  (%s · %s)", skill.Name, status, scope)
		if skill.Summary != "" {
			line += "\n    " + skill.Summary
		}
		a.ui.Println(line)
	}
}

func (a *App) printSessions() error {
	items, err := a.manager.List()
	if err != nil {
		return err
	}
	for _, item := range items {
		a.ui.Println(fmt.Sprintf("- %s  %s  %s", item.ID, item.Mode, item.UpdatedAt.Format(time.RFC3339)))
	}
	return nil
}

// commandArgument returns everything after the command word, preserving the
// original spacing and case of the task description.
func commandArgument(input, command string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), command))
}

// newBackgroundAgent builds an isolated agent for one background task. Each task
// gets its own session, its own tool registry, and a non-interactive policy so a
// detached agent can never block waiting for terminal input.
func (a *App) newBackgroundAgent(id string, out *background.Writer) (background.Job, error) {
	cfg := a.config.Get()
	providerConfig, ok := cfg.ActiveProviderConfig()
	if !ok {
		return background.Job{}, errors.New("no active provider; use /addprovider")
	}
	if cfg.ActiveModel == "" {
		return background.Job{}, errors.New("no active model; use /model")
	}
	client, err := provider.New(providerConfig)
	if err != nil {
		return background.Job{}, err
	}
	child, err := a.manager.New(a.project.Root)
	if err != nil {
		return background.Job{}, err
	}
	child.Mode = a.session.Mode
	if err := a.manager.Save(child); err != nil {
		return background.Job{}, err
	}
	// Background agents inherit YOLO but never prompt: a nil prompt makes the
	// policy deny anything that would have required an interactive answer.
	policy := safety.New(a.options.YOLO, nil)
	registry := tools.New(a.project.Root, policy, child, a.manager.Save, a.skills, out)
	codingAgent := agent.New(client, a.config, a.project, child, a.manager.Save, registry, out)
	codingAgent.SetNotes("You are running as detached background agent " + id + ". Nobody can answer questions or grant permissions, so never call ask_user. " +
		"Finish autonomously and end with a short report of what you changed and what remains. " +
		"In Ask mode every write and command is denied, so report what you would change instead of retrying denied tools.")
	return background.Job{Runner: codingAgent, Session: child.ID, Mode: child.Mode}, nil
}

func (a *App) onBackgroundEvent(event background.Event) {
	switch event.Status {
	case background.StatusDone:
		a.ui.Success(fmt.Sprintf("Background %s finished: %s · /bglog %s", event.ID, event.Label, event.ID))
	case background.StatusFailed:
		a.ui.Warn(fmt.Sprintf("Background %s failed: %s · %s", event.ID, event.Label, event.Err))
	case background.StatusCancelled:
		a.ui.Info(fmt.Sprintf("Background %s cancelled: %s", event.ID, event.Label))
	}
	a.ui.SetBackground(a.background.Running(), a.background.Limit())
}

func (a *App) spawnBackground(ctx context.Context, prompt string) error {
	if strings.TrimSpace(prompt) == "" {
		return errors.New("usage: /bg <task description>")
	}
	snapshot, err := a.background.Spawn(ctx, prompt)
	if err != nil {
		return err
	}
	a.ui.SetBackground(a.background.Running(), a.background.Limit())
	a.ui.Success(fmt.Sprintf("Background %s started (%d/%d): %s", snapshot.ID, a.background.Running(), a.background.Limit(), snapshot.Label))
	a.ui.Info("Session " + snapshot.Session + " · /bgs lists agents · /bglog " + snapshot.ID + " shows output")
	return nil
}

func (a *App) printBackground() {
	items := a.background.List()
	if len(items) == 0 {
		a.ui.Println(fmt.Sprintf("No background agents. Use /bg <task> to start one (limit %d).", a.background.Limit()))
		return
	}
	a.ui.Println(fmt.Sprintf("Background agents: %d running of %d allowed", a.background.Running(), a.background.Limit()))
	for _, item := range items {
		line := fmt.Sprintf("- %s  %-9s  %5s  %s", item.ID, item.Status, item.Duration().Truncate(time.Second), item.Label)
		if item.Err != "" {
			line += "  · " + background.Label(item.Err)
		}
		a.ui.Println(line)
	}
}

func (a *App) printBackgroundLog(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: /bglog <agent-id>")
	}
	snapshot, err := a.background.Get(args[0])
	if err != nil {
		return err
	}
	lines, err := a.background.Output(args[0])
	if err != nil {
		return err
	}
	a.ui.Println(fmt.Sprintf("%s · %s · %s · session %s", snapshot.ID, snapshot.Status, snapshot.Duration().Truncate(time.Second), snapshot.Session))
	a.ui.Println("Task: " + snapshot.Prompt)
	if snapshot.Err != "" {
		a.ui.Warn(snapshot.Err)
	}
	if len(lines) == 0 {
		a.ui.Println("No output captured yet.")
		return nil
	}
	for _, line := range lines {
		a.ui.Println("  " + line)
	}
	return nil
}

func (a *App) cancelBackground(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: /bgstop <agent-id|all>")
	}
	if strings.EqualFold(args[0], "all") {
		stopped := a.background.CancelAll()
		a.ui.SetBackground(a.background.Running(), a.background.Limit())
		a.ui.Success(fmt.Sprintf("Cancellation requested for %d agent(s).", stopped))
		return nil
	}
	snapshot, err := a.background.Cancel(args[0])
	if err != nil {
		return err
	}
	a.ui.Success("Cancellation requested for " + snapshot.ID + ": " + snapshot.Label)
	return nil
}

func (a *App) readTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	raw, _ := json.Marshal(arguments)
	return a.tools.Execute(ctx, core.ToolCall{ID: "cli", Name: name, Arguments: raw})
}

func (a *App) refreshStaleModels(ctx context.Context, force bool) {
	if !force && time.Since(a.lastRefresh) < time.Minute {
		return
	}
	a.lastRefresh = time.Now()
	cfg := a.config.Get()
	providers := append([]config.Provider(nil), cfg.Providers...)
	sort.Slice(providers, func(i, j int) bool { return providers[i].Name < providers[j].Name })
	for _, item := range providers {
		stale := item.ModelsFetchedAt.IsZero() || time.Since(item.ModelsFetchedAt) >= time.Duration(cfg.RefreshHours)*time.Hour
		if !stale {
			continue
		}
		refreshCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := a.refreshProviderModels(refreshCtx, item.Name)
		cancel()
		if err != nil && force {
			a.ui.Warn("Model refresh failed for " + item.Name + ": " + err.Error())
		}
	}
}

func checkpointLabel(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	var out strings.Builder
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
		} else if out.Len() > 0 && !strings.HasSuffix(out.String(), "-") {
			out.WriteByte('-')
		}
		if out.Len() >= 36 {
			break
		}
	}
	label := strings.Trim(out.String(), "-")
	if label == "" {
		label = "user-turn"
	}
	return "before-" + label
}

func (a *App) refreshProviderModels(ctx context.Context, name string) error {
	cfg := a.config.Get()
	providerConfig, ok := cfg.Provider(name)
	if !ok {
		return fmt.Errorf("provider %q not found", name)
	}
	client, err := provider.New(providerConfig)
	if err != nil {
		return err
	}
	models, err := client.ListModels(ctx)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return errors.New("provider returned an empty model list")
	}
	return a.config.Update(func(current *config.Config) error {
		for i := range current.Providers {
			if current.Providers[i].Name == name {
				current.Providers[i].Models = models
				current.Providers[i].ModelsFetchedAt = time.Now()
				if current.ActiveProvider == name && current.ActiveModel == "" {
					current.ActiveModel = models[0].ID
				}
				return nil
			}
		}
		return fmt.Errorf("provider %q disappeared", name)
	})
}

const helpText = `Commands
  /build              Switch to Build mode
  /plan               Switch to Plan mode
  /reverse            Switch to Reverse mode (analyze binaries and obfuscated code)
  /mode               Cycle Build → Plan → Reverse
  /addprovider        Add or replace an OpenAI-compatible or Anthropic provider
  /providers          Select a provider
  /model              Select an imported model
  /compact            Compact the current context now
  /autocompact        Enable or disable automatic compaction at 90%
  /context            Show estimated context usage
  /cost               Show token usage and configured cost estimate
  /diff               Show the current git diff
  /undo               Undo the latest Neko file write
  /checkpoint [name]  Create a manual restore point
  /restore            Roll back files and conversation to the latest checkpoint
  /checkpoints        List available restore points
  /skills             List configured skills and which mode each applies to
  /addskill            Register a local skill directory
  /addskills [dir]     Reinstall bundled skills for this mode, or add your own from a directory
  /permissions        Show the active permission policy
  /session            Show the current session ID
  /sessions           List sessions for this project
  /bg <task>          Start a detached background agent (up to 25 at once)
  /bgs                List background agents and their status
  /bglog <id>         Show the captured output of one background agent
  /bgstop <id|all>    Cancel one background agent or every running one
  /exit                Exit Neko

Keyboard
  Tab                  Cycle Build → Plan → Reverse

Startup flags
  neko --continue              Continue the latest session and replay its history
  neko --resume <session-id>   Resume a specific session and replay its history
  neko --continue --yolo       Continue and auto-approve allowed actions
  neko --background-jobs <n>   Cap concurrent background agents (1-25, default 25)`
