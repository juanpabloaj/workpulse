package collect

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSortProjectFilesByModTime(t *testing.T) {
	now := time.Now()
	paths := []string{
		"/tmp/older.jsonl",
		"/tmp/newest.jsonl",
		"/tmp/middle.jsonl",
	}
	modTimes := map[string]time.Time{
		"/tmp/older.jsonl":  now.Add(-10 * time.Minute),
		"/tmp/newest.jsonl": now.Add(-1 * time.Minute),
		"/tmp/middle.jsonl": now.Add(-5 * time.Minute),
	}

	sortProjectFilesByModTime(paths, modTimes)

	want := []string{
		"/tmp/newest.jsonl",
		"/tmp/middle.jsonl",
		"/tmp/older.jsonl",
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("sorted paths = %v, want %v", paths, want)
	}
}

func TestClaudeReadProjectSessionUsesCacheWhenMtimeUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	content := `{"timestamp":"2026-04-08T10:00:00Z","cwd":"/home/user/src/project","message":{"role":"assistant","content":[{"type":"text","text":"first"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	collector := &ClaudeCollector{
		root:         dir,
		sessionCache: make(map[string]claudeSessionCacheEntry),
	}

	session, ok := collector.readProjectSession(context.Background(), path)
	if !ok {
		t.Fatal("readProjectSession() = false, want true")
	}

	entry := collector.sessionCache[path]
	entry.session.Name = "cached session"
	collector.sessionCache[path] = entry

	cached, ok := collector.readProjectSession(context.Background(), path)
	if !ok {
		t.Fatal("second readProjectSession() = false, want true")
	}
	if cached.Name != "cached session" {
		t.Fatalf("cached session name = %q, want %q", cached.Name, "cached session")
	}
	if cached.ID != session.ID {
		t.Fatalf("cached session ID = %q, want %q", cached.ID, session.ID)
	}
}

func TestEncodeClaudeProject(t *testing.T) {
	got := encodeClaudeProject("/home/user/src/project_name-with-hyphen")
	want := "-home-user-src-project-name-with-hyphen"
	if got != want {
		t.Fatalf("encodeClaudeProject() = %q, want %q", got, want)
	}
}
