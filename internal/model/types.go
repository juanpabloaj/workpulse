package model

import "time"

type AgentKind string

const (
	AgentClaude AgentKind = "claude"
	AgentCodex  AgentKind = "codex"
	AgentGemini AgentKind = "gemini"
)

type SessionState string

const (
	StateRunning SessionState = "running"
	StateIdle    SessionState = "idle"
	StateBlocked SessionState = "blocked"
	StateError   SessionState = "error"
	StateDone    SessionState = "done"
	StateUnknown SessionState = "unknown"
)

type SessionOrigin string

const (
	OriginLiveIndex  SessionOrigin = "live-index"
	OriginTranscript SessionOrigin = "transcript"
	OriginMerged     SessionOrigin = "merged"
)

type ProcessInfo struct {
	PID     int
	PPID    int
	CPU     float64
	RSSMB   int
	Elapsed string
	Status  string
	TTY     string
	Command string
	Args    string
	CWD     string
}

type UsageStats struct {
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheCreationTokens int
}

type ToolStats struct {
	CurrentTool    string
	LastTool       string
	ToolCalls      int
	ToolErrors     int
	ToolSuccesses  int
	PermissionDeny int
}

type Session struct {
	ID           string
	Agent        AgentKind
	Name         string
	Project      string
	CWD          string
	Branch       string
	Model        string
	Provider     string
	Version      string
	StartedAt    time.Time
	UpdatedAt    time.Time
	LastEventAt  time.Time
	LastEvent    string
	LastUserText string
	State        SessionState
	Origin       SessionOrigin
	Subagents    int
	Process      *ProcessInfo
	Usage        UsageStats
	Tools        ToolStats
	Source       string
}

type Snapshot struct {
	CollectedAt time.Time
	Sessions    []Session
	Processes   []ProcessInfo
}
