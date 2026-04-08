# workpulse

> **Early stage software.** Expect breaking changes, incomplete data, and rough edges. Use at your own risk.

`workpulse` is a local-first terminal monitor (like htop) for coding agents.

The initial MVP focuses on:

- Claude sessions discovered from `~/.claude`
- Codex sessions discovered from `~/.codex`
- Gemini sessions discovered from `~/.gemini`
- OS-level process correlation
- A terminal UI that combines process data with session metadata, tool activity, and token usage

## Goals

`workpulse` is not trying to read agent reasoning. It is trying to expose operational truth:

- which agents are active
- which repo and branch they are working on
- which tool they are using
- whether tool calls are failing
- how many tokens they are using
- whether the process is still alive, idle, blocked, or done

## Data Sources

### Claude

- `~/.claude/sessions/*.json`
- `~/.claude/projects/<project>/<session>.jsonl`
- `~/.claude/projects/<project>/<session>/subagents/*.jsonl` for subagent counts only

### Codex

- `~/.codex/sessions/**/rollout-*.jsonl`

### Gemini

- `~/.gemini/tmp/<project-hash>/chats/session-*.json`
- `~/.gemini/projects.json` for project-to-path mapping
- `~/.gemini/history/*/.project_root` as fallback CWD resolution

### OS Process Data

- `ps`
- `lsof` for `cwd`

## MVP Scope

- discover local Claude, Codex, and Gemini sessions
- parse real metrics already available in local session artifacts
- correlate sessions with live processes when possible
- render a TUI with a main table and a detail pane

## Non-Goals

- no cloud service
- no MCP dependency
- no vendor instrumentation beyond what the local tools already write
- no inferred quality score in the main view

## Security Notes

Some local agent directories contain shell snapshots and other sensitive material.

`workpulse` should never render raw environment snapshots or secrets in the UI. The MVP intentionally avoids reading those files for display.

## Running

```bash
go run ./cmd/workpulse
```

## Current Limitations

- Codex live session correlation is heuristic because Codex does not expose a simple active-session PID index similar to Claude's `~/.claude/sessions/*.json`
- The current process collector is implemented around `ps` and `lsof`, which is practical for macOS and Linux but not yet tuned for Windows
- Token extraction is currently best-effort and depends on which fields are present in each local artifact
