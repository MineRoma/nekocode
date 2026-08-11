# Neko development instructions

- Keep dependencies focused: use Charm libraries for terminal UI and the Go standard library elsewhere.
- Preserve the default Ask permission mode and the hard safety blocks.
- Keep pasted provider keys only in the owner-readable local config; never write them to sessions, logs, or tool output.
- Keep all CLI labels and permission choices in English.
- Build mode may use every registered tool. Plan mode must expose read-only tools only.
- Run `go test ./...` after changes.
- New safety or path-validation behavior requires tests.
