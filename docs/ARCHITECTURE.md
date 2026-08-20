# Architecture

## Package map

```text
main.go                 flags, cancellation, process exit
internal/app            lifecycle, commands, provider wizard
internal/agent          model/tool loop, system prompt, compaction
internal/background     detached agent fleet, bounded output capture
internal/config         atomic JSON configuration
internal/core           protocol-neutral messages, tools, and session modes
internal/provider       OpenAI-compatible and Anthropic streaming adapters
internal/project        Git-root discovery, AGENTS.md, path containment
internal/session        UUID sessions and undo records
internal/safety         Ask/YOLO authorization and hard blocks
internal/tools          tool schemas and execution
internal/skills         local SKILL.md registry and embedded skill sets
internal/ui             persistent Bubble Tea session, viewport, modals, Lip Gloss styling
internal/diff           dependency-free line diff
```

Dependencies point toward `core`, `config`, and narrow interfaces. Provider-specific wire formats never enter the agent or tool layers.

## Agent turn

1. Refresh the system prompt with the current mode, project tree, `AGENTS.md`, and compacted memory.
2. Append and persist the user's message.
3. Send messages and mode-allowed tool schemas to the active provider.
4. Stream text deltas directly to the terminal.
5. Persist the assistant message and token usage.
6. Execute tool calls in order.
7. Append each tool result and repeat until the model returns no tools.
8. Keep submitted user turns, streaming assistant text, tool activity, and permission modals in one persistent viewport.
9. Stop after 24 model/tool iterations even in YOLO mode.

`--continue` and `--resume` additionally call `ui.Replay`, which renders the stored transcript into the viewport before the first prompt: user turns, assistant text, tool calls as collapsed one-liners, and the compaction summary when one exists. Replay is display-only — it does not touch `session.Messages`, which the model already receives in full.

Plan mode filters the registry to tools marked `ReadOnly`. Build and Reverse modes expose all tools.

## Modes

`core` owns the mode vocabulary: `ModeBuild`, `ModePlan`, `ModeReverse`, plus `NormalizeMode`, `NextMode`, and `ModeLabel`. Every other package routes mode decisions through those helpers rather than comparing strings, so adding a mode does not require finding scattered literals.

`NormalizeMode` maps unknown or empty values to Build. Sessions are normalized on load, so a session written by an older version — or hand-edited — still opens.

Reverse differs from Build in the system prompt only. It has the same tool surface because recovery work produces notes, scripts, and reimplementations; restricting it to read-only would make it useless for its own output.

## Skills

A skill directory holds `SKILL.md`, optionally prefixed with `---` delimited frontmatter (`name`, `mode`, `summary`). `skills.Discover` scans a parent directory for them, which is what `/addskills <dir>` uses to register a user's own set behind one permission prompt.

`Store.ForMode` returns skills tagged for the active mode plus untagged ones. The agent injects their names and summaries into the system prompt, so the model can choose a skill before spending a `load_skill` call. Skill bodies are not preloaded — only the index.

### Bundled skills

`internal/skills/bundled/<mode>/` is compiled into the binary with `go:embed`. `setMode` calls `InstallBundled`, which unpacks that mode's skills into the user config directory and registers any not already known.

Three properties this relies on:

- **Idempotent.** Existing config entries are left untouched, so a user's disabled flag or edited path survives. Only files are rewritten, which lets an upgraded binary refresh skill content.
- **Outside the project.** Unpacking targets the config directory, never the repository, so switching modes cannot dirty a working tree. That is also why it bypasses `safety.Policy` — no project file is involved.
- **Embedded fallback.** `Load` reads the unpacked file, and falls back to the embedded copy if the file was deleted, so a registered skill cannot break.

Build and Plan currently ship no bundled skills, so entering them registers nothing.

## Background agents

`internal/background` runs up to `MaxAgents` (25) detached agents per Neko session. The manager owns scheduling and state; it knows nothing about providers or tools.

- `app` supplies a `Factory` that builds one isolated agent per task: a fresh session from the same session manager, a fresh tool registry, and a `safety.Policy` with a nil prompt.
- A nil prompt makes `Authorize` return an error instead of blocking, so a detached agent can never wait on terminal input. In Ask mode this denies every write and command; in YOLO mode the agent proceeds.
- Each task holds a `context.CancelFunc` derived from the process context. `/bgstop` and shutdown cancel it, so agents stop at the next model or tool boundary.
- A `Writer` per task implements both the agent `UI` and the tool `Reporter` interfaces, capturing output into a ring buffer bounded to 500 lines and 4000 bytes per line.
- The manager holds one mutex over all task state and never calls into `app` while holding it. Completion callbacks run on the finishing goroutine after the lock is released.
- `ui.UI` guards its Bubble Tea program pointer with a mutex because background agents update the status counter from their own goroutines.

The foreground turn is unaffected: it keeps its own session, registry, and interactive policy.

## Permission boundary

Every mutating tool calls `safety.Policy.Authorize` immediately before the side effect.

- Ask mode prompts for once/session/deny.
- Session grants are keyed by action kind and exact resource.
- YOLO skips prompts.
- Hard blocks run before YOLO and cannot be overridden.
- File paths are resolved and checked against the project root before access.

Command execution is still a host shell operation. The policy is not an OS sandbox.

## Providers

The provider interface has two operations:

- `Chat`: protocol-neutral messages, tools, streaming callback, and usage.
- `ListModels`: imports models from `<base>/models`.

The OpenAI adapter uses streamed Chat Completions tool calls. The Anthropic adapter translates the same core messages into text, `tool_use`, and `tool_result` blocks.

## Persistence

Config is atomically replaced in the OS user config directory with owner-only permissions. Providers can reference an API-key environment variable or store a pasted key locally; secrets never enter sessions or tool output.

Sessions are isolated by a SHA-256 hash of the canonical project root, then stored by UUID. Each session persists after every message, tool result, plan update, and file change.

## Compaction

The context estimator uses approximately four UTF-8 bytes per token plus message overhead. At 90% of the configured model context window, Neko asks the active model for durable session memory, clears the old transcript, and injects the summary into future system prompts. `/compact` invokes the same mechanism manually.
