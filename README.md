# Neko

Neko is a Go terminal coding agent with a Claude Code-style interface, Build/Plan modes, reviewable file edits, safe command execution, resumable project sessions, skills, `AGENTS.md`, OpenAI-compatible APIs, and Anthropic.

> Version 0.4.1. Neko now uses a cyan-to-purple ASCII banner and mode-aware accents: blue for Build and orange for Plan.

![Neko terminal UI preview](docs/ui-preview.png)

## Core behavior

- **Build / Plan:** press `Tab` at an empty prompt, or use `/build` and `/plan`. Build uses a blue accent; Plan uses orange.
- **Ask by default:** writes, shell commands, and skill registration offer `Allow once`, `Allow for session`, or `Deny`.
- **YOLO:** `--yolo` auto-approves allowed actions. Hard-blocked machine-wide destructive commands remain blocked.
- **Project context:** Neko discovers the Git root, sends a bounded project tree, and loads global/root/nested `AGENTS.md` instructions.
- **Tools:** read, list, search, targeted replace, full write, shell, tests, Git diff, plans, and skills.
- **Verification loop:** the system prompt requires reading first, using tools, checking changed files, and running suitable tests.
- **Sessions:** each project has isolated UUID sessions with messages, plans, usage, summaries, and undo history.
- **Providers:** OpenAI Chat Completions-compatible streaming and Anthropic Messages streaming.
- **Model import:** models come from `<base-url>/models` and refresh after three hours.
- **Context control:** auto-compaction at 90% by default; `/autocompact` toggles it.
- **Claude Code-style UI:** one persistent full-screen session with a compact welcome card, scrollable conversation, persistent status line, `❯` input, slash-command hints, tool activity, thinking state, and boxed permission modals.
- **Visible user turns:** submitted prompts stay in the conversation as `❯ user message`, alongside assistant and tool output.
- **Clarification questions:** the model can present exactly three choices plus `Type my own answer…`.
- **Queued steering:** prompts entered while the agent works remain visible, run as the next user turn, and appear as a mode-colored queue count.
- **Searchable input:** `/` searches commands and `@` searches project files; arrows choose and Enter inserts.
- **Checkpoints:** every user task creates a restore point; `/checkpoint`, `/checkpoints`, and `/restore` manage rollbacks.
- **Compact tools:** tool results appear as one-line collapsed summaries.
- **Cross-platform input:** Bubble Tea handles Tab, arrows, Unicode, resizing, and Windows terminals.
- **English UI:** prompts, permissions, commands, and errors are in English.

## Install

Requires Go 1.22 or newer.

```bash
git clone <repository-url> neko-cli
cd neko-cli
go test ./...
go install .
```

Or build locally:

```bash
go build -o bin/neko .
./bin/neko
```

The first build downloads the Charm terminal UI libraries declared in `go.mod`. The resulting executable is a standalone binary.

## First run

Neko opens a provider wizard:

1. Select `OpenAI-compatible` or `Anthropic` with arrow keys.
2. Enter a provider name.
3. Enter a base URL that already includes `/v1` when required.
4. Choose authentication: paste an API key or provide an environment-variable name. Pasted keys are masked during entry and saved only in the owner-readable local config.
5. Neko imports models from `/models`; select one with arrow keys.

Examples:

```bash
export OPENAI_API_KEY='...'
neko

export ANTHROPIC_API_KEY='...'
neko
```

Default base URLs:

```text
OpenAI:   https://api.openai.com/v1
Anthropic: https://api.anthropic.com/v1
```

For an OpenAI-compatible gateway, use the gateway's full HTTPS `/v1` base URL. You can paste its key directly or reference an environment variable.

## Sessions

```bash
neko --continue
neko --resume 550e8400-e29b-41d4-a716-446655440000
neko --continue --yolo
```

- `--continue` opens the most recently updated session for the current project.
- `--resume <id>` opens a specific session in the current project.
- New sessions are created when neither flag is present.
- `/session` prints the active ID; `/sessions` lists project sessions.

## Permission model

### Ask mode (default)

Neko asks before writes, commands, or adding skills:

```text
Permission required
Run project tests
$ go test ./...

› Allow once
  Allow for session
  Deny
```

### YOLO mode

```bash
neko --yolo
```

YOLO permits file writes, `.env` changes, commands, `git push`, and skill registration without prompting. It does **not** bypass hard safety blocks.

Always blocked:

- `sudo`
- `rm -rf /`, equivalent root deletion, and `--no-preserve-root`
- `mkfs`
- `shutdown`, `reboot`, and `poweroff`
- shell fork bombs
- tool file access outside the project root

YOLO is intentionally powerful. Run it only in a disposable container, VM, or repository with a clean Git state.

## Commands

| Command | Action |
|---|---|
| `/build` | Switch to Build mode |
| `/plan` | Switch to Plan mode |
| `/mode` | Toggle Build/Plan |
| `/addprovider` | Add or replace a provider (`/addprovide` alias supported) |
| `/providers` | Select a provider |
| `/model` | Select an imported model |
| `/compact` | Compact context now |
| `/autocompact` | Enable or disable compaction at 90% |
| `/context` | Show estimated context usage |
| `/cost` | Show token usage and configured cost estimate |
| `/diff` | Show current Git diff |
| `/undo` | Undo the latest Neko file write |
| `/skills` | List skills |
| `/addskill` | Add a local skill directory |
| `/permissions` | Explain current permission mode |
| `/session` | Show active session ID |
| `/sessions` | List project sessions |
| `/exit` | Exit |

## `AGENTS.md`

Neko loads instructions in this order:

1. `~/.config/neko/AGENTS.md` (platform config directory)
2. `<git-root>/AGENTS.md`
3. Nested `AGENTS.md` files from the root to the current directory

Example:

```md
# Project instructions

- Use PostgreSQL.
- Do not change public APIs without asking.
- Run `go test ./...` after edits.
- Add tests for new behavior.
```

Instructions become part of every model turn, including resumed sessions.

## Skills

A skill is a local directory containing `SKILL.md`.

```text
my-skill/
└── SKILL.md
```

Register it interactively:

```text
/addskill review-security ./skills/review-security
```

The user must approve skill registration in Ask mode. The agent can then call `load_skill` when the task matches.

## Storage

- Config: the OS user config directory under `neko/config.json`
- Sessions: the OS user cache directory under `neko/projects/<project-hash>/sessions`
- API keys: either an environment variable or the owner-readable local config when pasted

Config and session files use owner-only directory permissions where supported.

## Development

```bash
make test
make build
```

See `docs/ARCHITECTURE.md` for package boundaries and the implemented agent loop.

## Known MVP limits

- Neko enables true-color rendering through Bubble Tea on CMD, PowerShell, and Windows Terminal. Windows Terminal still provides the best font and Unicode rendering.
- Cost is shown only when model pricing is manually present in config; token usage always works.
- The built-in diff is full-file rather than hunk-collapsed.
- Model APIs differ across gateways. If `/models` is unsupported, Neko asks for a model ID.
- Command isolation is policy-based, not an OS sandbox. Use a container for hostile repositories or YOLO mode.

## License

MIT
