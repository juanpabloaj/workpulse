package collect

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/juanpabloaj/workpulse/internal/model"
)

func TestCodexReadRolloutUsesCacheWhenMtimeUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout.jsonl")
	content := `{"timestamp":"2026-04-08T10:00:00Z","type":"session_meta","payload":{"id":"session-1","cwd":"/home/user/src/project","cli_version":"1.0.0","model_provider":"openai"}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	collector := &CodexCollector{
		root:         dir,
		sessionCache: make(map[string]codexSessionCacheEntry),
	}

	session, ok := collector.readRollout(context.Background(), path)
	if !ok {
		t.Fatal("readRollout() = false, want true")
	}

	entry := collector.sessionCache[path]
	entry.session.Name = "cached session"
	collector.sessionCache[path] = entry

	cached, ok := collector.readRollout(context.Background(), path)
	if !ok {
		t.Fatal("second readRollout() = false, want true")
	}
	if cached.Name != "cached session" {
		t.Fatalf("cached session name = %q, want %q", cached.Name, "cached session")
	}
	if cached.ID != session.ID {
		t.Fatalf("cached session ID = %q, want %q", cached.ID, session.ID)
	}
	if cached.Agent != model.AgentCodex {
		t.Fatalf("cached session agent = %q, want %q", cached.Agent, model.AgentCodex)
	}
}
