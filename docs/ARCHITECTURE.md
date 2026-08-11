# Architecture

## Package map

```text
main.go                 flags, cancellation, process exit
internal/app            lifecycle, commands, provider wizard
internal/agent          model/tool loop, system prompt, compaction
internal/config         atomic JSON configuration
internal/core           protocol-neutral messages and tools
internal/provider       OpenAI-compatible and Anthropic streaming adapters
internal/project        Git-root discovery, AGENTS.md, path containment
internal/session        UUID sessions and undo records
internal/safety         Ask/YOLO authorization and hard blocks
internal/tools          tool schemas and execution
internal/skills         local SKILL.md registry
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

Plan mode filters the registry to tools marked `ReadOnly`. Build mode exposes all tools.

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
