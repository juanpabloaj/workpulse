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

type ClaudeCollector struct {
	root         string
	mu           sync.RWMutex
	sessionCache map[string]claudeSessionCacheEntry
}

func NewClaudeCollector() *ClaudeCollector {
	return &ClaudeCollector{
		root:         homePath(".claude"),
		sessionCache: make(map[string]claudeSessionCacheEntry),
	}
}

type claudeSessionCacheEntry struct {
	session model.Session
	mtime   time.Time
}

type claudeSessionIndex struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	CWD        string `json:"cwd"`
	StartedAt  int64  `json:"startedAt"`
	Kind       string `json:"kind"`
	Entrypoint string `json:"entrypoint"`
	Name       string `json:"name"`
}

type claudeMessage struct {
	Type      string             `json:"type"`
	Timestamp string             `json:"timestamp"`
	CWD       string             `json:"cwd"`
	SessionID string             `json:"sessionId"`
	Version   string             `json:"version"`
	GitBranch string             `json:"gitBranch"`
	AgentID   string             `json:"agentId"`
	Message   *claudeMessageBody `json:"message"`
	UserType  string             `json:"userType"`
}

type claudeMessageBody struct {
	Role    string               `json:"role"`
	Content []claudeContentBlock `json:"content"`
	Model   string               `json:"model"`
	Usage   *claudeUsage         `json:"usage"`
}

type claudeContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	IsError   bool            `json:"is_error"`
	ToolUseID string          `json:"tool_use_id"`
	Content   string          `json:"content"`
}

type claudeUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	CacheReadInput     int `json:"cache_read_input_tokens"`
	CacheCreationInput int `json:"cache_creation_input_tokens"`
}

func (c *ClaudeCollector) Collect(ctx context.Context) ([]model.Session, error) {
	indexFiles, err := filepath.Glob(filepath.Join(c.root, "sessions", "*.json"))
	if err != nil {
		return nil, err
	}

	sessionByID := map[string]model.Session{}
	for _, path := range indexFiles {
		session, ok := c.readIndexedSession(ctx, path)
		if !ok {
			continue
		}
		sessionByID[session.ID] = session
	}

	projectFiles, err := filepath.Glob(filepath.Join(c.root, "projects", "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}

	projectModTimes := make(map[string]time.Time, len(projectFiles))
	for _, path := range projectFiles {
		if info, statErr := os.Stat(path); statErr == nil {
			projectModTimes[path] = info.ModTime()
		}
	}
	sortProjectFilesByModTime(projectFiles, projectModTimes)

	if len(projectFiles) > 20 {
		projectFiles = projectFiles[:20]
	}

	for _, path := range projectFiles {
		session, ok := c.readProjectSession(ctx, path)
		if !ok {
			continue
		}
		existing, exists := sessionByID[session.ID]
		if !exists {
			sessionByID[session.ID] = session
			continue
		}
		sessionByID[session.ID] = mergeClaudeSessions(existing, session)
	}

	sessions := make([]model.Session, 0, len(sessionByID))
	for _, session := range sessionByID {
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastEventAt.After(sessions[j].LastEventAt)
	})
	return sessions, nil
}

func sortProjectFilesByModTime(paths []string, modTimes map[string]time.Time) {
	sort.Slice(paths, func(i, j int) bool {
		iMod, iOK := modTimes[paths[i]]
		jMod, jOK := modTimes[paths[j]]
		if !iOK || !jOK {
			return paths[i] < paths[j]
		}
		return iMod.After(jMod)
	})
}

func (c *ClaudeCollector) readIndexedSession(_ context.Context, path string) (model.Session, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return model.Session{}, false
	}

	var index claudeSessionIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		return model.Session{}, false
	}

	projectDir := filepath.Join(c.root, "projects", encodeClaudeProject(index.CWD))
	transcriptPath := filepath.Join(projectDir, fmt.Sprintf("%s.jsonl", index.SessionID))
	if info, err := os.Stat(transcriptPath); err == nil {
		if cached, ok := c.cachedClaudeSession(transcriptPath, info.ModTime()); ok {
			cached.ID = index.SessionID
			cached.Agent = model.AgentClaude
			cached.Name = firstNonEmpty(index.Name, index.Kind, cached.Name, "Claude session")
			cached.Project = shortenPath(index.CWD)
			cached.CWD = index.CWD
			cached.StartedAt = parseUnixMillis(index.StartedAt)
			cached.UpdatedAt = parseUnixMillis(index.StartedAt)
			cached.Process = &model.ProcessInfo{PID: index.PID}
			cached.Origin = model.OriginLiveIndex
			cached.Source = transcriptPath
			return cached, true
		}
	}

	lines, err := tailLines(transcriptPath, 400)
	if err != nil {
		return model.Session{}, false
	}

	session := model.Session{
		ID:        index.SessionID,
		Agent:     model.AgentClaude,
		Name:      firstNonEmpty(index.Name, index.Kind, "Claude session"),
		Project:   shortenPath(index.CWD),
		CWD:       index.CWD,
		StartedAt: parseUnixMillis(index.StartedAt),
		UpdatedAt: parseUnixMillis(index.StartedAt),
		Process:   &model.ProcessInfo{PID: index.PID},
		Origin:    model.OriginLiveIndex,
		Source:    transcriptPath,
	}

	session.Subagents = countClaudeSubagents(projectDir, index.SessionID)
	for _, line := range lines {
		var msg claudeMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		updateClaudeSession(&session, msg)
	}

	if session.LastEventAt.IsZero() {
		session.LastEventAt = session.StartedAt
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.LastEventAt
	}
	if info, err := os.Stat(transcriptPath); err == nil {
		c.mu.Lock()
		c.sessionCache[transcriptPath] = claudeSessionCacheEntry{session: session, mtime: info.ModTime()}
		c.mu.Unlock()
	}
	return session, true
}

func (c *ClaudeCollector) readProjectSession(_ context.Context, path string) (model.Session, bool) {
	if info, err := os.Stat(path); err == nil {
		if cached, ok := c.cachedClaudeSession(path, info.ModTime()); ok {
			return cached, cached.ID != ""
		}
	}

	lines, err := tailLines(path, 400)
	if err != nil || len(lines) == 0 {
		return model.Session{}, false
	}

	sessionID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	session := model.Session{
		ID:     sessionID,
		Agent:  model.AgentClaude,
		Name:   "Claude session",
		Origin: model.OriginTranscript,
		Source: path,
	}

	for _, line := range lines {
		var msg claudeMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		updateClaudeSession(&session, msg)
	}

	if session.CWD == "" {
		session.CWD = decodeClaudeProject(filepath.Base(filepath.Dir(path)))
		session.Project = shortenPath(session.CWD)
	}
	session.Subagents = countClaudeSubagents(filepath.Dir(path), sessionID)
	if session.LastEventAt.IsZero() {
		if info, err := os.Stat(path); err == nil {
			session.LastEventAt = info.ModTime()
			session.UpdatedAt = info.ModTime()
		}
	}
	if info, err := os.Stat(path); err == nil {
		c.mu.Lock()
		c.sessionCache[path] = claudeSessionCacheEntry{session: session, mtime: info.ModTime()}
		c.mu.Unlock()
	}
	return session, session.ID != ""
}

func (c *ClaudeCollector) cachedClaudeSession(path string, modTime time.Time) (model.Session, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.sessionCache[path]
	if !ok || !entry.mtime.Equal(modTime) {
		return model.Session{}, false
	}
	return entry.session, true
}

func updateClaudeSession(session *model.Session, msg claudeMessage) {
	if msg.CWD != "" {
		session.CWD = msg.CWD
		session.Project = shortenPath(msg.CWD)
	}
	if msg.GitBranch != "" {
		session.Branch = msg.GitBranch
	}
	if msg.Version != "" {
		session.Version = msg.Version
	}
	if ts, err := time.Parse(time.RFC3339, msg.Timestamp); err == nil {
		session.LastEventAt = ts
		session.UpdatedAt = ts
	}

	if msg.AgentID != "" {
		session.Subagents++
	}

	if msg.Message == nil {
		return
	}
	if msg.Message.Model != "" {
		session.Model = msg.Message.Model
		session.Provider = "anthropic"
	}
	if usage := msg.Message.Usage; usage != nil {
		session.Usage.InputTokens += usage.InputTokens
		session.Usage.OutputTokens += usage.OutputTokens
		session.Usage.CacheReadTokens += usage.CacheReadInput
		session.Usage.CacheCreationTokens += usage.CacheCreationInput
		if strings.HasPrefix(session.Model, "claude-") {
			totalInput := usage.InputTokens + usage.CacheReadInput + usage.CacheCreationInput
			session.Usage.ContextWindow = 200000
			session.Usage.ContextPct = percentOfWindow(totalInput, session.Usage.ContextWindow)
		}
	}

	for _, block := range msg.Message.Content {
		switch block.Type {
		case "text":
			text := strings.TrimSpace(block.Text)
			if text != "" {
				session.LastEvent = trimForDisplay(text)
				if msg.Message.Role == "user" {
					session.LastUserText = trimForDisplay(text)
				}
			}
		case "tool_use":
			session.Tools.ToolCalls++
			session.Tools.LastTool = firstNonEmpty(block.Name, "tool")
			session.Tools.CurrentTool = session.Tools.LastTool
			session.LastEvent = fmt.Sprintf("tool: %s", session.Tools.LastTool)
		case "tool_result":
			session.Tools.CurrentTool = ""
			if block.IsError {
				session.Tools.ToolErrors++
				session.LastEvent = trimForDisplay(block.Content)
				if containsAnyLower(block.Content, "denied", "permission", "approval") {
					session.Tools.PermissionDeny++
				}
			} else {
				session.Tools.ToolSuccesses++
			}
		}
	}
}

func countClaudeSubagents(projectDir, sessionID string) int {
	pattern := filepath.Join(projectDir, sessionID, "subagents", "*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return 0
	}
	return len(matches)
}

func encodeClaudeProject(cwd string) string {
	return strings.ReplaceAll(cwd, string(filepath.Separator), "-")
}

func decodeClaudeProject(project string) string {
	if project == "" {
		return ""
	}
	if strings.HasPrefix(project, "-") {
		project = strings.TrimPrefix(project, "-")
		return string(filepath.Separator) + strings.ReplaceAll(project, "-", string(filepath.Separator))
	}
	return strings.ReplaceAll(project, "-", string(filepath.Separator))
}

func mergeClaudeSessions(primary, secondary model.Session) model.Session {
	merged := primary
	merged.Origin = model.OriginMerged
	if merged.Name == "" || merged.Name == "Claude session" {
		merged.Name = firstNonEmpty(secondary.Name, merged.Name)
	}
	merged.Project = firstNonEmpty(merged.Project, secondary.Project)
	merged.CWD = firstNonEmpty(merged.CWD, secondary.CWD)
	merged.Branch = firstNonEmpty(merged.Branch, secondary.Branch)
	merged.Model = firstNonEmpty(merged.Model, secondary.Model)
	merged.Provider = firstNonEmpty(merged.Provider, secondary.Provider)
	merged.Version = firstNonEmpty(merged.Version, secondary.Version)
	merged.Source = firstNonEmpty(merged.Source, secondary.Source)
	if merged.LastEventAt.Before(secondary.LastEventAt) {
		merged.LastEventAt = secondary.LastEventAt
		merged.UpdatedAt = secondary.UpdatedAt
		merged.LastEvent = firstNonEmpty(secondary.LastEvent, merged.LastEvent)
		merged.LastUserText = firstNonEmpty(secondary.LastUserText, merged.LastUserText)
	}
	if merged.Process == nil && secondary.Process != nil {
		merged.Process = secondary.Process
	}
	if merged.Subagents == 0 {
		merged.Subagents = secondary.Subagents
	}
	if merged.Tools.ToolCalls == 0 {
		merged.Tools = secondary.Tools
	}
	if merged.Usage.InputTokens == 0 && merged.Usage.OutputTokens == 0 {
		merged.Usage = secondary.Usage
	}
	return merged
}
