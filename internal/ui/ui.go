package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/m1neroma/neko/internal/core"
	"github.com/m1neroma/neko/internal/safety"
	"github.com/muesli/termenv"
)

var ErrInterrupted = errors.New("interrupted")

var slashCommands = []string{
	"/build", "/plan", "/mode", "/model", "/providers", "/addprovider",
	"/compact", "/autocompact", "/context", "/cost", "/diff", "/undo",
	"/checkpoint", "/checkpoints", "/restore",
	"/skills", "/addskill", "/permissions", "/session", "/sessions", "/help", "/exit",
}

type Input struct {
	Text        string
	ToggleMode  bool
	Interrupted bool
}

type UI struct {
	in          *bufio.Reader
	out         io.Writer
	interactive bool
	color       bool
	mode        string
	yolo        bool
	model       string
	session     string
	cwd         string
	program     *tea.Program
	input       chan Input
	done        chan error
}

func New() *UI {
	cwd, _ := os.Getwd()
	interactive := isTerminal(os.Stdin) && isTerminal(os.Stdout)
	color := interactive && os.Getenv("NO_COLOR") == ""
	if color {
		// Bubble Tea enables virtual terminal processing on supported Windows consoles.
		// Force the same true-color palette in CMD, PowerShell, and Windows Terminal.
		lipgloss.SetColorProfile(termenv.TrueColor)
	}
	return &UI{
		in: bufio.NewReader(os.Stdin), out: os.Stdout,
		interactive: interactive, color: color, cwd: cwd,
		input: make(chan Input, 32), done: make(chan error, 1),
	}
}

func (u *UI) Header(mode string, yolo bool, model, session string) {
	u.mode, u.yolo, u.model, u.session = mode, yolo, model, session
	if !u.interactive {
		fmt.Fprintln(u.out, "\n✦ Neko Code")
		fmt.Fprintln(u.out, fallback(model, "No model selected"))
		fmt.Fprintln(u.out, filepath.Clean(u.cwd))
		return
	}
	if u.program == nil {
		chat := newChatModel(mode, yolo, model, session, u.cwd, u.input, u.color)
		u.program = tea.NewProgram(chat, tea.WithAltScreen(), tea.WithInput(os.Stdin), tea.WithOutput(u.out))
		go func() {
			_, err := u.program.Run()
			u.done <- err
		}()
		return
	}
	u.program.Send(statusMsg{mode: mode, yolo: yolo, model: model, session: session})
}

func (u *UI) Close() {
	if u.program == nil {
		return
	}
	u.program.Send(closeMsg{})
	<-u.done
	u.program = nil
}

func (u *UI) ReadInput(mode string) (Input, error) {
	u.mode = mode
	if u.program == nil {
		fmt.Fprint(u.out, u.plainStatus()+"\n❯ ")
		line, err := u.in.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return Input{}, err
		}
		return Input{Text: strings.TrimSpace(line)}, err
	}
	select {
	case result := <-u.input:
		if result.Interrupted {
			return Input{}, ErrInterrupted
		}
		return result, nil
	case err := <-u.done:
		u.program = nil
		if err != nil {
			return Input{}, err
		}
		return Input{}, ErrInterrupted
	}
}

func (u *UI) Select(title string, options []string, selected int) (int, error) {
	if len(options) == 0 {
		return -1, errors.New("no options")
	}
	if selected < 0 || selected >= len(options) {
		selected = 0
	}
	if u.program != nil {
		response := make(chan selectResult, 1)
		u.program.Send(openSelectMsg{title: title, options: options, selected: selected, response: response})
		result := <-response
		if result.cancelled {
			return -1, ErrInterrupted
		}
		return result.selected, nil
	}
	if !u.interactive {
		fmt.Fprintln(u.out, title)
		for i, option := range options {
			fmt.Fprintf(u.out, "  %d) %s\n", i+1, option)
		}
		fmt.Fprint(u.out, "> ")
		line, err := u.in.ReadString('\n')
		if err != nil {
			return -1, err
		}
		n, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || n < 1 || n > len(options) {
			return -1, errors.New("invalid selection")
		}
		return n - 1, nil
	}
	model := standaloneMenu{title: title, options: options, cursor: selected, color: u.color}
	program := tea.NewProgram(model, tea.WithInput(os.Stdin), tea.WithOutput(u.out))
	final, err := program.Run()
	if err != nil {
		return -1, err
	}
	result, ok := final.(standaloneMenu)
	if !ok || result.cancelled {
		return -1, ErrInterrupted
	}
	return result.cursor, nil
}

func (u *UI) Prompt(label string) (string, error) {
	return u.runPrompt(label, false)
}

func (u *UI) PromptSecret(label string) (string, error) {
	return u.runPrompt(label, true)
}

func (u *UI) runPrompt(label string, secret bool) (string, error) {
	if u.program != nil {
		response := make(chan promptResult, 1)
		u.program.Send(openPromptMsg{label: label, secret: secret, response: response})
		result := <-response
		if result.cancelled {
			return "", ErrInterrupted
		}
		return strings.TrimSpace(result.value), nil
	}
	if !u.interactive {
		fmt.Fprint(u.out, label+" ")
		line, err := u.in.ReadString('\n')
		return strings.TrimSpace(line), err
	}
	model := newStandaloneInput(label, secret, u.color)
	program := tea.NewProgram(model, tea.WithInput(os.Stdin), tea.WithOutput(u.out))
	final, err := program.Run()
	if err != nil {
		return "", err
	}
	result, ok := final.(standaloneInput)
	if !ok || result.cancelled {
		return "", ErrInterrupted
	}
	return strings.TrimSpace(result.value), nil
}

func (u *UI) Permission(action safety.Action) safety.Decision {
	options := []string{"Allow once", "Allow for session", "Deny"}
	if u.program != nil {
		response := make(chan selectResult, 1)
		u.program.Send(openSelectMsg{
			title: "Permission required", body: action.Description + "\n\n" + compactPreview(action.Preview, 12, 96),
			options: options, selected: 0, response: response, permission: true,
		})
		result := <-response
		if result.cancelled {
			return safety.Deny
		}
		return []safety.Decision{safety.AllowOnce, safety.AllowSession, safety.Deny}[result.selected]
	}
	fmt.Fprintln(u.out, "\nPermission required\n"+action.Description)
	if action.Preview != "" {
		fmt.Fprintln(u.out, compactPreview(action.Preview, 12, 96))
	}
	choice, err := u.Select("Allow this action?", options, 0)
	if err != nil {
		return safety.Deny
	}
	return []safety.Decision{safety.AllowOnce, safety.AllowSession, safety.Deny}[choice]
}

func (u *UI) Ask(question string, options []core.QuestionOption) (string, error) {
	display := make([]string, 0, len(options)+1)
	for _, option := range options {
		line := option.Label
		if option.Description != "" {
			line += " — " + option.Description
		}
		display = append(display, line)
	}
	display = append(display, "Type my own answer…")
	choice, err := u.Select(question, display, 0)
	if err != nil {
		return "", err
	}
	if choice == len(options) {
		answer, err := u.Prompt("Your answer")
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(answer) == "" {
			return "", errors.New("answer cannot be empty")
		}
		return answer, nil
	}
	return options[choice].Label, nil
}

func (u *UI) Tool(name, detail string) {
	labels := map[string]string{
		"read_file": "Read", "list_files": "Explore", "search": "Search", "git_diff": "Diff",
		"write_file": "Write", "replace_in_file": "Edit", "run_command": "Bash", "run_tests": "Test",
		"update_plan": "Plan", "ask_user": "Question", "load_skill": "Skill", "add_skill": "Add skill",
	}
	label := labels[name]
	if label == "" {
		label = name
	}
	if detail != "" {
		label += "(" + detail + ")"
	}
	u.log("tool", "● "+label)
}

func (u *UI) ToolResult(name, result string, failed bool) {
	prefix := "└─ "
	if failed {
		prefix = "└─ error: "
	}
	u.log("tool", prefix+summarizeToolResult(result)+" · output collapsed")
}

func (u *UI) Thinking() {
	if u.program != nil {
		u.program.Send(thinkingMsg(true))
		return
	}
	fmt.Fprint(u.out, "✦ Thinking…")
}

func (u *UI) Stream(text string) {
	if u.program != nil {
		u.program.Send(streamMsg(text))
		return
	}
	fmt.Fprint(u.out, text)
}

func (u *UI) EndStream() {
	if u.program != nil {
		u.program.Send(endStreamMsg{})
		return
	}
	fmt.Fprintln(u.out)
}

func (u *UI) Info(text string)    { u.log("info", "● "+text) }
func (u *UI) Success(text string) { u.log("success", "✓ "+text) }
func (u *UI) Warn(text string)    { u.log("warn", "! "+text) }
func (u *UI) Error(err error)     { u.log("error", "× "+err.Error()) }
func (u *UI) Println(text string) { u.log("plain", text) }

func (u *UI) log(kind, text string) {
	if u.program != nil {
		u.program.Send(logMsg{kind: kind, text: text})
		return
	}
	fmt.Fprintln(u.out, text)
}

func (u *UI) plainStatus() string {
	permission := "ASK"
	if u.yolo {
		permission = "YOLO"
	}
	return strings.ToLower(fallback(u.mode, "build")) + " · " + fallback(u.model, "no model") + " · " + permission
}

type logEntry struct {
	kind string
	text string
}

type chatModel struct {
	width, height    int
	mode             string
	yolo             bool
	model            string
	session          string
	cwd              string
	color            bool
	viewport         viewport.Model
	input            textinput.Model
	submit           chan Input
	logs             []logEntry
	files            []string
	suggestionCursor int
	thinking         bool
	streamIndex      int
	modal            *modalState
}

type modalState struct {
	kind       string
	title      string
	body       string
	options    []string
	cursor     int
	input      textinput.Model
	selectResp chan selectResult
	promptResp chan promptResult
	permission bool
}

type statusMsg struct {
	mode, model, session string
	yolo                 bool
}
type logMsg struct{ kind, text string }
type thinkingMsg bool
type streamMsg string
type endStreamMsg struct{}
type closeMsg struct{}
type selectResult struct {
	selected  int
	cancelled bool
}
type promptResult struct {
	value     string
	cancelled bool
}
type openSelectMsg struct {
	title, body string
	options     []string
	selected    int
	response    chan selectResult
	permission  bool
}
type openPromptMsg struct {
	label    string
	secret   bool
	response chan promptResult
}

func newChatModel(mode string, yolo bool, model, session, cwd string, submit chan Input, color bool) chatModel {
	field := textinput.New()
	field.Prompt = "❯ "
	field.Placeholder = "Describe a task or type /help"
	field.CharLimit = 32 * 1024
	field.Width = 90
	field.Focus()
	view := viewport.New(80, 16)
	return chatModel{
		width: 90, height: 28, mode: mode, yolo: yolo, model: model, session: session,
		cwd: cwd, color: color, viewport: view, input: field, submit: submit, streamIndex: -1,
		files: collectProjectFiles(cwd, 2500),
	}
}

func (m chatModel) Init() tea.Cmd { return textinput.Blink }

func (m chatModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeViewport()
		return m, nil
	case closeMsg:
		return m, tea.Quit
	case statusMsg:
		m.mode, m.yolo, m.model, m.session = msg.mode, msg.yolo, msg.model, msg.session
		return m, nil
	case logMsg:
		m.logs = append(m.logs, logEntry{kind: msg.kind, text: msg.text})
		m.refreshLogs()
		return m, nil
	case thinkingMsg:
		m.thinking = bool(msg)
		m.refreshLogs()
		return m, nil
	case streamMsg:
		m.thinking = false
		if m.streamIndex < 0 {
			m.logs = append(m.logs, logEntry{kind: "assistant", text: string(msg)})
			m.streamIndex = len(m.logs) - 1
		} else {
			m.logs[m.streamIndex].text += string(msg)
		}
		m.refreshLogs()
		return m, nil
	case endStreamMsg:
		m.thinking = false
		m.streamIndex = -1
		m.refreshLogs()
		return m, nil
	case openSelectMsg:
		m.modal = &modalState{
			kind: "select", title: msg.title, body: msg.body, options: msg.options, cursor: msg.selected,
			selectResp: msg.response, permission: msg.permission,
		}
		return m, nil
	case openPromptMsg:
		field := textinput.New()
		field.Prompt = "❯ "
		field.Placeholder = msg.label
		field.CharLimit = 32 * 1024
		field.Width = maxInt(30, minInt(90, m.width-8))
		if msg.secret {
			field.EchoMode = textinput.EchoPassword
			field.EchoCharacter = '•'
		}
		field.Focus()
		m.modal = &modalState{kind: "prompt", title: msg.label, input: field, promptResp: msg.response}
		return m, textinput.Blink
	case tea.KeyMsg:
		if m.modal != nil {
			return m.updateModal(msg)
		}
		suggestions := inputSuggestions(m.input.Value(), m.files, 5)
		if len(suggestions) > 0 {
			switch msg.String() {
			case "up":
				m.suggestionCursor = (m.suggestionCursor - 1 + len(suggestions)) % len(suggestions)
				return m, nil
			case "down":
				m.suggestionCursor = (m.suggestionCursor + 1) % len(suggestions)
				return m, nil
			case "enter":
				m.input.SetValue(applySuggestion(m.input.Value(), suggestions[m.suggestionCursor]))
				m.suggestionCursor = 0
				return m, nil
			}
		}
		switch msg.String() {
		case "ctrl+c":
			m.submit <- Input{Interrupted: true}
			return m, tea.Quit
		case "tab":
			m.submit <- Input{ToggleMode: true}
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			if value != "" {
				m.logs = append(m.logs, logEntry{kind: "user", text: value})
				m.input.SetValue("")
				m.refreshLogs()
				m.submit <- Input{Text: value}
			}
			return m, nil
		}
	}
	if m.modal == nil {
		var inputCmd, viewportCmd tea.Cmd
		m.input, inputCmd = m.input.Update(message)
		m.viewport, viewportCmd = m.viewport.Update(message)
		return m, tea.Batch(inputCmd, viewportCmd)
	}
	return m, nil
}

func (m chatModel) updateModal(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modal.kind == "select" {
		switch key.String() {
		case "ctrl+c", "esc":
			m.modal.selectResp <- selectResult{cancelled: true}
			m.modal = nil
		case "up", "k":
			m.modal.cursor = (m.modal.cursor - 1 + len(m.modal.options)) % len(m.modal.options)
		case "down", "j":
			m.modal.cursor = (m.modal.cursor + 1) % len(m.modal.options)
		case "enter":
			m.modal.selectResp <- selectResult{selected: m.modal.cursor}
			m.modal = nil
		}
		return m, nil
	}
	switch key.String() {
	case "ctrl+c", "esc":
		m.modal.promptResp <- promptResult{cancelled: true}
		m.modal = nil
		return m, nil
	case "enter":
		m.modal.promptResp <- promptResult{value: m.modal.input.Value()}
		m.modal = nil
		return m, nil
	}
	var command tea.Cmd
	m.modal.input, command = m.modal.input.Update(key)
	return m, command
}

func (m *chatModel) resizeViewport() {
	m.viewport.Width = maxInt(20, m.width-4)
	headerHeight := lipgloss.Height(m.headerView())
	m.viewport.Height = maxInt(4, m.height-headerHeight-5)
	m.input.Width = maxInt(20, m.width-6)
	m.refreshLogs()
}

func (m *chatModel) refreshLogs() {
	m.viewport.SetContent(m.logsView())
	m.viewport.GotoBottom()
}

func (m chatModel) logsView() string {
	var out strings.Builder
	for i, entry := range m.logs {
		if i > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(m.renderLog(entry))
	}
	if m.thinking {
		if len(m.logs) > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(m.paint(lipgloss.NewStyle().Foreground(lipgloss.Color("#7D7A75")), "✦ Thinking…"))
	}
	return out.String()
}

func (m chatModel) renderLog(entry logEntry) string {
	switch entry.kind {
	case "user":
		return m.paint(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E6E5E3")), "❯ "+entry.text)
	case "assistant":
		return m.paint(lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E5E3")), "✦ "+entry.text)
	case "tool":
		return m.paint(lipgloss.NewStyle().Foreground(lipgloss.Color("#9A9894")), entry.text)
	case "success":
		return m.paint(lipgloss.NewStyle().Foreground(lipgloss.Color("#72BC8F")), entry.text)
	case "warn":
		return m.paint(lipgloss.NewStyle().Foreground(lipgloss.Color("#D19A66")), entry.text)
	case "error":
		return m.paint(lipgloss.NewStyle().Foreground(lipgloss.Color("#E06C75")), entry.text)
	case "info":
		return m.paint(lipgloss.NewStyle().Foreground(lipgloss.Color("#61AFEF")), entry.text)
	default:
		return entry.text
	}
}

func (m chatModel) View() string {
	header := m.headerView()
	status := m.statusView()
	if m.modal != nil {
		return header + "\n\n" + m.modalView() + "\n\n" + status
	}
	suggestions := inputSuggestions(m.input.Value(), m.files, 5)
	var hint strings.Builder
	for i, suggestion := range suggestions {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#7D7A75"))
		if i == m.suggestionCursor {
			prefix = "❯ "
			style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D97757"))
		}
		hint.WriteString("\n" + prefix)
		hint.WriteString(m.paint(style, suggestion))
	}
	return header + "\n\n" + m.viewport.View() + "\n" + status + "\n" + m.input.View() + hint.String()
}

func (m chatModel) headerView() string {
	width := maxInt(30, minInt(108, m.width-4))
	logo := "✦ NEKO CODE"
	if m.width >= 104 {
		logo = strings.Join(nekoCodeBanner, "\n")
	}
	logo = m.gradient(logo)
	body := logo + "\n" + m.muted(fallback(m.model, "No model selected")) + "\n" + m.muted(filepath.Clean(m.cwd))
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(width)
	if m.color {
		style = style.BorderForeground(lipgloss.Color("#D97757"))
	}
	return style.Render(body)
}

func (m chatModel) statusView() string {
	permission := "ASK"
	if m.yolo {
		permission = "YOLO"
	}
	value := strings.ToLower(fallback(m.mode, "build")) + " · " + fallback(m.model, "no model") + " · " + permission + " · Tab switches mode"
	return m.muted(value)
}

func (m chatModel) modalView() string {
	width := maxInt(30, minInt(76, m.width-4))
	border := lipgloss.Color("#D97757")
	if m.modal.permission {
		border = lipgloss.Color("#D19A66")
	}
	title := m.paint(lipgloss.NewStyle().Bold(true).Foreground(border), m.modal.title)
	var body strings.Builder
	body.WriteString(title)
	if m.modal.body != "" {
		body.WriteString("\n")
		body.WriteString(m.muted(m.modal.body))
	}
	if m.modal.kind == "prompt" {
		body.WriteString("\n\n")
		body.WriteString(m.modal.input.View())
	} else {
		body.WriteString("\n\n")
		for i, option := range m.modal.options {
			prefix := "  "
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("#9A9894"))
			if i == m.modal.cursor {
				prefix = "❯ "
				style = lipgloss.NewStyle().Bold(true).Foreground(border)
			}
			body.WriteString(m.paint(style, prefix+option))
			body.WriteByte('\n')
		}
		body.WriteString(m.muted("↑/↓ move · Enter select · Esc cancel"))
	}
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Width(width)
	if m.color {
		style = style.BorderForeground(border)
	}
	return style.Render(body.String())
}

func (m chatModel) gradient(value string) string {
	if !m.color {
		return value
	}
	runes := []rune(value)
	visible := 0
	for _, r := range runes {
		if r != '\n' && r != ' ' {
			visible++
		}
	}
	var out strings.Builder
	index := 0
	for _, r := range runes {
		if r == '\n' || r == ' ' {
			out.WriteRune(r)
			continue
		}
		ratio := float64(index) / float64(maxInt(1, visible-1))
		red := int(255 + float64(192-255)*ratio)
		green := int(122 + float64(132-122)*ratio)
		blue := int(89 + float64(252-89)*ratio)
		color := lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", red, green, blue))
		out.WriteString(lipgloss.NewStyle().Bold(true).Foreground(color).Render(string(r)))
		index++
	}
	return out.String()
}

func (m chatModel) muted(value string) string {
	return m.paint(lipgloss.NewStyle().Foreground(lipgloss.Color("#7D7A75")), value)
}

func (m chatModel) paint(style lipgloss.Style, value string) string {
	if !m.color {
		return value
	}
	return style.Render(value)
}

type standaloneInput struct {
	input     textinput.Model
	value     string
	cancelled bool
	color     bool
}

func newStandaloneInput(label string, secret, color bool) standaloneInput {
	field := textinput.New()
	field.Prompt = "❯ "
	field.Placeholder = label
	field.CharLimit = 32 * 1024
	field.Width = 90
	if secret {
		field.EchoMode = textinput.EchoPassword
		field.EchoCharacter = '•'
	}
	field.Focus()
	return standaloneInput{input: field, color: color}
}

func (m standaloneInput) Init() tea.Cmd { return textinput.Blink }
func (m standaloneInput) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			m.value = m.input.Value()
			return m, tea.Quit
		}
	}
	var command tea.Cmd
	m.input, command = m.input.Update(message)
	return m, command
}
func (m standaloneInput) View() string { return m.input.View() }

type standaloneMenu struct {
	title     string
	options   []string
	cursor    int
	cancelled bool
	color     bool
}

func (m standaloneMenu) Init() tea.Cmd { return nil }
func (m standaloneMenu) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			m.cursor = (m.cursor - 1 + len(m.options)) % len(m.options)
		case "down", "j":
			m.cursor = (m.cursor + 1) % len(m.options)
		case "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}
func (m standaloneMenu) View() string {
	var out strings.Builder
	title := m.render(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D97757")), m.title)
	out.WriteString(title + "\n")
	for i, option := range m.options {
		prefix := "  "
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#7D7A75"))
		if i == m.cursor {
			prefix = "❯ "
			style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#D97757"))
		}
		out.WriteString(m.render(style, prefix+option) + "\n")
	}
	out.WriteString(m.render(lipgloss.NewStyle().Foreground(lipgloss.Color("#7D7A75")), "↑/↓ move · Enter select · Esc cancel"))
	return out.String()
}
func (m standaloneMenu) render(style lipgloss.Style, value string) string {
	if !m.color {
		return value
	}
	return style.Render(value)
}

var nekoCodeBanner = []string{
	"███╗   ██╗ ███████╗ ██╗  ██╗  ██████╗      ██████╗  ██████╗  ██████╗  ███████╗",
	"████╗  ██║ ██╔════╝ ██║ ██╔╝ ██╔═══██╗    ██╔════╝ ██╔═══██╗ ██╔══██╗ ██╔════╝",
	"██╔██╗ ██║ █████╗   █████╔╝  ██║   ██║    ██║      ██║   ██║ ██║  ██║ █████╗  ",
	"██║╚██╗██║ ██╔══╝   ██╔═██╗  ██║   ██║    ██║      ██║   ██║ ██║  ██║ ██╔══╝  ",
	"██║ ╚████║ ███████╗ ██║  ██╗ ╚██████╔╝    ╚██████╗ ╚██████╔╝ ██████╔╝ ███████╗",
	"╚═╝  ╚═══╝ ╚══════╝ ╚═╝  ╚═╝  ╚═════╝      ╚═════╝  ╚═════╝  ╚═════╝  ╚══════╝",
}

func applySuggestion(value, suggestion string) string {
	if strings.HasPrefix(value, "/") && !strings.Contains(value, " ") {
		return suggestion + " "
	}
	index := strings.LastIndex(value, "@")
	if index < 0 {
		return value
	}
	return value[:index] + suggestion + " "
}

func inputSuggestions(value string, files []string, limit int) []string {
	if strings.HasPrefix(value, "/") && !strings.Contains(value, " ") {
		return commandMatches(value, limit)
	}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return nil
	}
	token := fields[len(fields)-1]
	if !strings.HasPrefix(token, "@") {
		return nil
	}
	query := strings.ToLower(strings.TrimPrefix(token, "@"))
	var matches []string
	for _, file := range files {
		if query == "" || strings.Contains(strings.ToLower(file), query) {
			matches = append(matches, "@"+file)
			if len(matches) == limit {
				break
			}
		}
	}
	return matches
}

func collectProjectFiles(root string, limit int) []string {
	ignored := map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "target": true}
	var files []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() && path != root && ignored[entry.Name()] {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err == nil {
			files = append(files, filepath.ToSlash(rel))
		}
		if len(files) >= limit {
			return fs.SkipAll
		}
		return nil
	})
	return files
}

func commandMatches(value string, limit int) []string {
	if !strings.HasPrefix(value, "/") || strings.Contains(value, " ") {
		return nil
	}
	var matches []string
	for _, command := range slashCommands {
		if strings.HasPrefix(command, value) && command != value {
			matches = append(matches, command)
			if len(matches) == limit {
				break
			}
		}
	}
	return matches
}

func summarizeToolResult(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "completed"
	}
	line := strings.TrimSpace(strings.Split(value, "\n")[0])
	if len(line) > 88 {
		line = line[:88] + "…"
	}
	count := strings.Count(value, "\n") + 1
	if count > 1 {
		line += fmt.Sprintf(" (%d lines)", count)
	}
	return line
}

func compactPreview(value string, maxLines, maxWidth int) string {
	if value == "" {
		return ""
	}
	lines := strings.Split(value, "\n")
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], "… preview truncated …")
	}
	for i, line := range lines {
		if len(line) > maxWidth {
			lines[i] = line[:maxWidth] + "…"
		}
	}
	return strings.Join(lines, "\n")
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func fallback(value, alternate string) string {
	if value == "" {
		return alternate
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
