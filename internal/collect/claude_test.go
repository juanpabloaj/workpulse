package collect

import (
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
