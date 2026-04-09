package collect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeminiReadChatUsesCacheWhenMtimeUnchanged(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "tmp", "project-hash", "chats")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	path := filepath.Join(projectDir, "session-1.json")
	content := `{"sessionId":"session-1","projectHash":"project-hash","startTime":"2026-04-08T10:00:00Z","lastUpdated":"2026-04-08T10:00:00Z","messages":[{"timestamp":"2026-04-08T10:00:00Z","type":"gemini","content":"[{\"text\":\"hello\"}]","model":"gemini-2.5"}]}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	collector := &GeminiCollector{
		root:         dir,
		sessionCache: make(map[string]geminiSessionCacheEntry),
	}

	session, ok := collector.readChat(path, map[string]string{"project-hash": "/home/user/src/project"})
	if !ok {
		t.Fatal("readChat() = false, want true")
	}

	entry := collector.sessionCache[path]
	entry.session.Name = "cached session"
	collector.sessionCache[path] = entry

	cached, ok := collector.readChat(path, map[string]string{"project-hash": "/home/user/src/project"})
	if !ok {
		t.Fatal("second readChat() = false, want true")
	}
	if cached.Name != "cached session" {
		t.Fatalf("cached session name = %q, want %q", cached.Name, "cached session")
	}
	if cached.ID != session.ID {
		t.Fatalf("cached session ID = %q, want %q", cached.ID, session.ID)
	}
}
