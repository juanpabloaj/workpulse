package collect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/juanpabloaj/workpulse/internal/model"
)

type CodexCollector struct {
	root         string
	mu           sync.RWMutex
	sessionCache map[string]codexSessionCacheEntry
}

func NewCodexCollector() *CodexCollector {
	return &CodexCollector{
		root:         homePath(".codex"),
		sessionCache: make(map[string]codexSessionCacheEntry),
	}
}

type codexSessionCacheEntry struct {
	session model.Session
	mtime   time.Time
}

type codexEnvelope struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexSessionMeta struct {
	ID            string `json:"id"`
	CWD           string `json:"cwd"`
	CLI           string `json:"cli_version"`
	ModelProvider string `json:"model_provider"`
}

type codexResponseItem struct {
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []codexContent `json:"content"`
	Name    string         `json:"name"`
}

type codexContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexEventMsg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Phase   string `json:"phase"`
}

type codexTokenCount struct {
	Info struct {
		TotalTokenUsage struct {
			InputTokens           int `json:"input_tokens"`
			OutputTokens          int `json:"output_tokens"`
			CachedInputTokens     int `json:"cached_input_tokens"`
			ReasoningOutputTokens int `json:"reasoning_output_tokens"`
		} `json:"total_token_usage"`
		LastTokenUsage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"last_token_usage"`
		ModelContextWindow int `json:"model_context_window"`
	} `json:"info"`
	RateLimits struct {
		Primary struct {
			UsedPercent   float64 `json:"used_percent"`
			WindowMinutes int     `json:"window_minutes"`
			ResetsAt      int64   `json:"resets_at"`
		} `json:"primary"`
		Secondary struct {
			UsedPercent   float64 `json:"used_percent"`
			WindowMinutes int     `json:"window_minutes"`
			ResetsAt      int64   `json:"resets_at"`
		} `json:"secondary"`
	} `json:"rate_limits"`
}

func (c *CodexCollector) Collect(ctx context.Context) ([]model.Session, error) {
	root := filepath.Join(c.root, "sessions")
	files, err := listRecentFiles(root, ".jsonl", 25)
	if err != nil {
		return nil, err
	}

	sort.Strings(files)

	var sessions []model.Session
	for _, path := range files {
		session, ok := c.readRollout(ctx, path)
		if !ok {
			continue
		}
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (c *CodexCollector) readRollout(_ context.Context, path string) (model.Session, bool) {
	if info, err := os.Stat(path); err == nil {
		if cached, ok := c.cachedCodexSession(path, info.ModTime()); ok {
			return cached, true
		}
	}

	lines, err := tailLines(path, 400)
	if err != nil || len(lines) == 0 {
		return model.Session{}, false
	}

	head, err := firstLine(path)
	if err == nil && strings.TrimSpace(head) != "" && head != lines[0] {
		lines = append([]string{head}, lines...)
	}

	session := model.Session{
		Agent:  model.AgentCodex,
		Origin: model.OriginTranscript,
		Source: path,
	}

	for _, line := range lines {
		var env codexEnvelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			continue
		}
		updateCodexSession(&session, env)
	}

	if session.ID == "" {
		return model.Session{}, false
	}
	if session.Name == "" {
		session.Name = "Codex session"
	}
	if session.Project == "" {
		session.Project = shortenPath(session.CWD)
	}
	if session.LastEventAt.IsZero() {
		session.LastEventAt = session.UpdatedAt
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now()
	}
	if info, err := os.Stat(path); err == nil {
		c.mu.Lock()
		c.sessionCache[path] = codexSessionCacheEntry{session: session, mtime: info.ModTime()}
		c.mu.Unlock()
	}
	return session, true
}

func (c *CodexCollector) cachedCodexSession(path string, modTime time.Time) (model.Session, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.sessionCache[path]
	if !ok || !entry.mtime.Equal(modTime) {
		return model.Session{}, false
	}
	return entry.session, true
}

func updateCodexSession(session *model.Session, env codexEnvelope) {
	if ts, err := time.Parse(time.RFC3339, env.Timestamp); err == nil {
		session.UpdatedAt = ts
		session.LastEventAt = ts
	}

	switch env.Type {
	case "turn_context":
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(env.Payload, &payload); err == nil && payload.Model != "" {
			session.Model = payload.Model
		}
	case "session_meta":
		var meta struct {
			Payload codexSessionMeta `json:"payload"`
		}
		// Unused: session_meta is already in env.Payload.
		_ = meta

		var payload codexSessionMeta
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return
		}
		session.ID = payload.ID
		session.CWD = payload.CWD
		session.Project = shortenPath(payload.CWD)
		session.Provider = payload.ModelProvider
		session.Version = payload.CLI
		session.Name = "Codex session"
	case "response_item":
		var payload codexResponseItem
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return
		}
		switch payload.Type {
		case "message":
			text := codexContentText(payload.Content)
			if payload.Role == "user" {
				session.LastUserText = trimForDisplay(text)
				if session.Name == "" && text != "" {
					session.Name = trimForDisplay(text)
				}
			} else if text != "" {
				session.Tools.CurrentTool = ""
				session.LastEvent = trimForDisplay(text)
			}
		case "function_call":
			session.Tools.ToolCalls++
			session.Tools.CurrentTool = payload.Name
			session.Tools.LastTool = payload.Name
			session.LastEvent = fmt.Sprintf("tool: %s", payload.Name)
		}
	case "event_msg":
		var payload codexEventMsg
		if err := json.Unmarshal(env.Payload, &payload); err != nil {
			return
		}
		switch payload.Type {
		case "exec_command_end":
			if containsAnyLower(payload.Message, "failed", "error", "denied") {
				session.Tools.ToolErrors++
			}
		case "agent_message", "user_message":
			if payload.Message != "" {
				session.LastEvent = trimForDisplay(payload.Message)
			}
			if payload.Type == "agent_message" {
				session.Tools.CurrentTool = ""
			}
		case "token_count":
			var tokens codexTokenCount
			if err := json.Unmarshal(env.Payload, &tokens); err == nil {
				tokenTS := session.LastEventAt
				if session.CodexRateLimitUpdatedAt.After(tokenTS) {
					tokenTS = session.CodexRateLimitUpdatedAt
				}
				if tokens.Info.TotalTokenUsage.InputTokens > 0 {
					session.Usage.InputTokens = tokens.Info.TotalTokenUsage.InputTokens
					session.Usage.OutputTokens = tokens.Info.TotalTokenUsage.OutputTokens
					session.Usage.CacheReadTokens = tokens.Info.TotalTokenUsage.CachedInputTokens
				}
				if tokens.Info.ModelContextWindow > 0 {
					session.Usage.ContextWindow = tokens.Info.ModelContextWindow
					session.Usage.ContextPct = percentOfWindow(tokens.Info.LastTokenUsage.InputTokens, tokens.Info.ModelContextWindow)
				}
				if tokens.RateLimits.Primary.ResetsAt > 0 || tokens.RateLimits.Primary.UsedPercent > 0 {
					session.CodexRateLimitPrimary = model.RateLimitWindow{
						UsedPercent: tokens.RateLimits.Primary.UsedPercent,
						ResetsAt:    time.Unix(tokens.RateLimits.Primary.ResetsAt, 0),
					}
					session.CodexRateLimitUpdatedAt = tokenTS
				}
				if tokens.RateLimits.Secondary.ResetsAt > 0 || tokens.RateLimits.Secondary.UsedPercent > 0 {
					session.CodexRateLimitSecondary = model.RateLimitWindow{
						UsedPercent: tokens.RateLimits.Secondary.UsedPercent,
						ResetsAt:    time.Unix(tokens.RateLimits.Secondary.ResetsAt, 0),
					}
					session.CodexRateLimitUpdatedAt = tokenTS
				}
			}
		}
	}
}

func codexContentText(content []codexContent) string {
	var parts []string
	for _, item := range content {
		if item.Text != "" {
			parts = append(parts, item.Text)
		}
	}
	return strings.Join(parts, " ")
}
