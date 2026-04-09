package collect

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/juanpabloaj/workpulse/internal/model"
)

func TestParseBatchCWDOutput(t *testing.T) {
	out := strings.Join([]string{
		"p101",
		"fcwd",
		"n/home/user/src/project1",
		"p202",
		"f1",
		"n/dev/null",
		"fcwd",
		"n/home/user/src/project2",
		"f2",
		"n/tmp/other",
		"p303",
		"",
	}, "\n")

	got := parseBatchCWDOutput(out)
	want := map[int]string{
		101: "/home/user/src/project1",
		202: "/home/user/src/project2",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseBatchCWDOutput() = %#v, want %#v", got, want)
	}
}

func TestProcessCollectorCollectUsesCache(t *testing.T) {
	expected := []model.ProcessInfo{
		{PID: 101, Command: "claude", CWD: "/home/user/src/project1"},
	}
	collector := &ProcessCollector{
		cache:    append([]model.ProcessInfo(nil), expected...),
		cachedAt: time.Now(),
		cacheTTL: 5 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("Collect() = %#v, want %#v", got, expected)
	}
}
