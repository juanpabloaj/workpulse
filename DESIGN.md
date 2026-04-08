# workpulse Design

## Product Thesis

Agent tooling already exposes partial views:

- process monitors show CPU and memory but not session meaning
- transcript viewers show messages but not live process state
- observability platforms show traces but often require instrumentation or a web dashboard

`workpulse` combines two planes:

1. session semantics already emitted by local agent tools
2. live operating system process state

That combination is the core value.

## Core Principles

- local-first
- terminal-first
- real metrics before inferred scores
- adapter-based ingestion
- explicit separation between observed facts and heuristics

## Observed Facts

Examples of facts that should be displayed directly:

- PID
- CPU and memory
- working directory
- session ID
- model and provider
- tool call count
- tool errors
- token usage
- subagent count
- last event timestamp

## Heuristics

Heuristics are still useful and are currently presented in the UI as derived state:

- running
- idle
- blocked
- done

These states are derived from a combination of live process presence, recent transcript events, and known error or permission-denial signals.

## Architecture

### Adapters

Each adapter is responsible for a single vendor-specific local artifact format.

- `ClaudeCollector`
- `CodexCollector`

Adapters normalize vendor-specific events into a shared session model.

### OS Probe

The process collector gathers:

- PID / PPID
- CPU
- RSS
- elapsed time
- TTY
- command and args
- current working directory

### Correlation

Correlation is currently performed in this order:

1. direct PID match when available
2. current working directory match

### TUI

The TUI presents:

- a main table for all sessions
- a detail panel for the selected session
- a compact help footer

## Safety

Some local artifacts, especially shell snapshots, may contain secrets.

`workpulse` should treat those files as sensitive input and avoid rendering them directly.
