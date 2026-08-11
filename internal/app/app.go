package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/m1neroma/neko/internal/agent"
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
	Continue bool
	Resume   string
	YOLO     bool
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
	tools       *tools.Registry
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
	}
	if err := a.ensureProvider(ctx); err != nil {
		return err
	}
	a.refreshStaleModels(ctx, true)
	a.ui.Header(a.session.Mode, options.YOLO, a.config.Get().ActiveModel, a.session.ID)
	defer a.ui.Close()
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
		a.setMode("build")
	case "/plan":
		a.setMode("plan")
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
	default:
		return true, false, fmt.Errorf("unknown command %s; use /help", command)
	}
	return true, false, nil
}

func (a *App) toggleMode() {
	if a.session.Mode == "plan" {
		a.setMode("build")
	} else {
		a.setMode("plan")
	}
}

func (a *App) setMode(mode string) {
	a.session.Mode = mode
	_ = a.manager.Save(a.session)
	a.ui.Header(mode, a.options.YOLO, a.config.Get().ActiveModel, a.session.ID)
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

func (a *App) printSkills() {
	items := a.skills.List()
	if len(items) == 0 {
		a.ui.Println("No skills configured. Use /addskill.")
		return
	}
	for _, skill := range items {
		status := "disabled"
		if skill.Enabled {
			status = "enabled"
		}
		a.ui.Println(fmt.Sprintf("- %s  %s  (%s)", skill.Name, skill.Path, status))
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
  /mode               Toggle Build/Plan
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
  /skills             List configured skills
  /addskill            Register a local skill directory
  /permissions        Show the active permission policy
  /session            Show the current session ID
  /sessions           List sessions for this project
  /exit                Exit Neko

Keyboard
  Tab                  Switch Build/Plan mode

Startup flags
  neko --continue              Continue the latest session in this project
  neko --resume <session-id>   Resume a specific session in this project
  neko --continue --yolo       Continue and auto-approve allowed actions`
