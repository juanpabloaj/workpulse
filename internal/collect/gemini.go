package collect

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/juanpabloaj/workpulse/internal/model"
)

type GeminiCollector struct {
	root         string
	mu           sync.RWMutex
	sessionCache map[string]geminiSessionCacheEntry
}

func NewGeminiCollector() *GeminiCollector {
	return &GeminiCollector{
		root:         homePath(".gemini"),
		sessionCache: make(map[string]geminiSessionCacheEntry),
	}
}

type geminiSessionCacheEntry struct {
	session model.Session
	mtime   time.Time
}

type geminiProjectsFile struct {
	Projects map[string]string `json:"projects"`
}

type geminiChatSession struct {
	SessionID   string          `json:"sessionId"`
	ProjectHash string          `json:"projectHash"`
	StartTime   string          `json:"startTime"`
	LastUpdated string          `json:"lastUpdated"`
	Messages    []geminiMessage `json:"messages"`
}

type geminiMessage struct {
	ID             string            `json:"id"`
	Timestamp      string            `json:"timestamp"`
	Type           string            `json:"type"`
	Content        json.RawMessage   `json:"content"`
	DisplayContent []geminiTextBlock `json:"displayContent"`
	Thoughts       []geminiThought   `json:"thoughts"`
	Tokens         *geminiTokens     `json:"tokens"`
	Model          string            `json:"model"`
	ToolCalls      []geminiToolCall  `json:"toolCalls"`
}

type geminiTextBlock struct {
	Text string `json:"text"`
}

type geminiThought struct {
	Subject     string `json:"subject"`
	Description string `json:"description"`
	Timestamp   string `json:"timestamp"`
}

type geminiTokens struct {
	Input    int `json:"input"`
	Output   int `json:"output"`
	Cached   int `json:"cached"`
	Thoughts int `json:"thoughts"`
	Tool     int `json:"tool"`
	Total    int `json:"total"`
}

type geminiToolCall struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	Timestamp   string `json:"timestamp"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
}

func (c *GeminiCollector) Collect(ctx context.Context) ([]model.Session, error) {
	_ = ctx

	chatFiles, err := c.listRecentChatFiles(25)
	if err != nil {
		return nil, err
	}

	projectRoots := c.loadGeminiProjectRoots()
	var sessions []model.Session
	for _, path := range chatFiles {
		session, ok := c.readChat(path, projectRoots)
		if !ok {
			continue
		}
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastEventAt.After(sessions[j].LastEventAt)
	})
	return sessions, nil
}

func (c *GeminiCollector) listRecentChatFiles(limit int) ([]string, error) {
	root := filepath.Join(c.root, "tmp")
	type entry struct {
		path    string
		modTime time.Time
	}
	var entries []entry

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !strings.Contains(path, string(filepath.Separator)+"chats"+string(filepath.Separator)+"session-") || filepath.Ext(path) != ".json" {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		entries = append(entries, entry{path: path, modTime: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].modTime.After(entries[j].modTime)
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.path)
	}
	return paths, nil
}

func (c *GeminiCollector) loadGeminiProjectRoots() map[string]string {
	roots := map[string]string{}

	projectsPath := filepath.Join(c.root, "projects.json")
	raw, err := os.ReadFile(projectsPath)
	if err == nil {
		var payload geminiProjectsFile
		if json.Unmarshal(raw, &payload) == nil {
			for cwd, projectName := range payload.Projects {
				roots[projectName] = cwd
			}
		}
	}

	historyRoots, err := filepath.Glob(filepath.Join(c.root, "history", "*", ".project_root"))
	if err == nil {
		for _, path := range historyRoots {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				continue
			}
			projectName := filepath.Base(filepath.Dir(path))
			cwd := strings.TrimSpace(string(data))
			if projectName != "" && cwd != "" {
				roots[projectName] = cwd
			}
		}
	}

	return roots
}

func (c *GeminiCollector) readChat(path string, projectRoots map[string]string) (model.Session, bool) {
	if info, err := os.Stat(path); err == nil {
		if cached, ok := c.cachedGeminiSession(path, info.ModTime()); ok {
			projectKey := filepath.Base(filepath.Dir(filepath.Dir(path)))
			cwd := projectRoots[projectKey]
			if cached.CWD == "" {
				cached.CWD = cwd
				cached.Project = firstNonEmpty(shortenPath(cwd), projectKey)
			}
			return cached, cached.ID != ""
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return model.Session{}, false
	}

	var chat geminiChatSession
	if err := json.Unmarshal(raw, &chat); err != nil {
		return model.Session{}, false
	}

	projectKey := filepath.Base(filepath.Dir(filepath.Dir(path)))
	cwd := projectRoots[projectKey]

	session := model.Session{
		ID:        chat.SessionID,
		Agent:     model.AgentGemini,
		Name:      "Gemini session",
		CWD:       cwd,
		Project:   firstNonEmpty(shortenPath(cwd), projectKey),
		StartedAt: parseTimeBestEffort(chat.StartTime),
		UpdatedAt: parseTimeBestEffort(chat.LastUpdated),
		Origin:    model.OriginTranscript,
		Source:    path,
		Provider:  "google",
	}

	for _, msg := range chat.Messages {
		updateGeminiSession(&session, msg)
	}

	if session.LastEventAt.IsZero() {
		session.LastEventAt = session.UpdatedAt
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.LastEventAt
	}
	if session.Project == "" || session.Project == "-" {
		session.Project = firstNonEmpty(projectKey, shortenPath(session.CWD), "-")
	}
	if info, err := os.Stat(path); err == nil {
		c.mu.Lock()
		c.sessionCache[path] = geminiSessionCacheEntry{session: session, mtime: info.ModTime()}
		c.mu.Unlock()
	}

	return session, session.ID != ""
}

func (c *GeminiCollector) cachedGeminiSession(path string, modTime time.Time) (model.Session, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.sessionCache[path]
	if !ok || !entry.mtime.Equal(modTime) {
		return model.Session{}, false
	}
	return entry.session, true
}

func updateGeminiSession(session *model.Session, msg geminiMessage) {
	if ts := parseTimeBestEffort(msg.Timestamp); !ts.IsZero() {
		session.LastEventAt = ts
		session.UpdatedAt = ts
	}

	switch msg.Type {
	case "user":
		session.Tools.CurrentTool = ""
		text := firstNonEmpty(geminiBlocksText(msg.DisplayContent), geminiContentText(msg.Content))
		if text != "" {
			session.LastUserText = trimForDisplay(text)
		}
	case "gemini":
		text := geminiContentText(msg.Content)
		if text != "" {
			session.Tools.CurrentTool = ""
			session.LastEvent = trimForDisplay(text)
		}
		if msg.Model != "" {
			session.Model = msg.Model
		}
		if msg.Tokens != nil {
			session.Usage.InputTokens += msg.Tokens.Input
			session.Usage.OutputTokens += msg.Tokens.Output
			session.Usage.CacheReadTokens += msg.Tokens.Cached
			if strings.HasPrefix(session.Model, "gemini-") {
				session.Usage.ContextWindow = 1048576
				session.Usage.ContextPct = percentOfWindow(msg.Tokens.Input, session.Usage.ContextWindow)
			}
		}
		for _, tool := range msg.ToolCalls {
			session.Tools.ToolCalls++
			session.Tools.LastTool = firstNonEmpty(tool.DisplayName, tool.Name, session.Tools.LastTool)
			session.Tools.CurrentTool = session.Tools.LastTool
			if tool.Status != "" && tool.Status != "success" {
				session.Tools.ToolErrors++
			}
			if ts := parseTimeBestEffort(tool.Timestamp); !ts.IsZero() && ts.After(session.LastEventAt) {
				session.LastEventAt = ts
				session.UpdatedAt = ts
			}
		}
		if len(msg.ToolCalls) > 0 && session.LastEvent == "" {
			session.LastEvent = "tool activity"
		}
	}
}

func geminiBlocksText(blocks []geminiTextBlock) string {
	var parts []string
	for _, block := range blocks {
		if strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, " ")
}

func geminiContentText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return plain
	}

	var blocks []geminiTextBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		return geminiBlocksText(blocks)
	}

	return ""
}
