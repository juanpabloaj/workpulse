# Repository Guidelines

## Project Structure & Module Organization

`workpulse` is a Go CLI/TUI application.

- `cmd/workpulse/`: application entrypoint
- `internal/collect/`: data collectors for Claude, Codex, OS processes, and correlation logic
- `internal/model/`: shared domain types
- `internal/tui/`: Bubble Tea model, layout, and rendering tests
- `README.md`, `DESIGN.md`: product intent and architecture notes

Keep new code inside `internal/` unless it is a user-facing entrypoint under `cmd/`.

## Build, Test, and Development Commands

- `go run ./cmd/workpulse`: run the TUI locally
- `go build ./...`: compile the full project
- `go test ./...`: run all tests
- `go test ./internal/tui -run TestTableHeaderRendered -v`: run the focused TUI regression test
- `gofmt -w ./cmd ./internal`: format source files

Run `gofmt` before submitting changes. Prefer small, verifiable iterations.

## Coding Style & Naming Conventions

Use standard Go formatting and idioms:

- tabs via `gofmt`; do not hand-align spacing
- exported names: `CamelCase`
- unexported helpers: `camelCase`
- keep packages focused (`collect`, `model`, `tui`)

Prefer explicit, descriptive names such as `ProcessCollector`, `SessionOrigin`, or `detailPanelOuterHeight`. Keep UI strings in English.

## Testing Guidelines

Use Go’s built-in `testing` package. Place tests next to the code they cover, following the `*_test.go` pattern.

Focus coverage on:

- transcript parsing edge cases
- session correlation and deduplication
- TUI layout regressions

Add a regression test for any bug involving rendering, filtering, or state classification.

## Commit & Pull Request Guidelines

The current history is minimal (`go mod init`), so use short imperative commit messages such as:

- `Add Claude transcript discovery`
- `Fix Codex live session correlation`
- `Tighten TUI layout spacing`

PRs should include:

- a concise description of the change
- why it was needed
- validation performed (`go build`, `go test`, manual TUI check)
- screenshots for visible TUI changes

## Security & Configuration Notes

Do not display secrets from local agent directories. Files under `~/.claude` and `~/.codex` may contain transcripts, shell snapshots, and sensitive environment data. Treat them as untrusted input and redact aggressively in UI-facing features.
