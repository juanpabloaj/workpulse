package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/juanpabloaj/workpulse/internal/model"
)

func TestTableHeaderRendered(t *testing.T) {
	m := NewModel(nil, nil, "dev", "unknown")
	m.snapshot = model.Snapshot{
		Sessions: []model.Session{
			{
				ID:        "1",
				Agent:     model.AgentClaude,
				Project:   "convdrift",
				State:     model.StateRunning,
				LastEvent: "ok",
			},
		},
	}
	m.filtered = m.snapshot.Sessions
	m.tableRows = 5

	out := m.tableView()
	for _, label := range []string{"St", "PID", "Agent", "State", "Last Event"} {
		if !strings.Contains(out, label) {
			t.Fatalf("table header label %q missing from output: %q", label, out)
		}
	}
}

func TestDetailViewKeepsFixedHeightWithLongLines(t *testing.T) {
	m := NewModel(nil, nil, "dev", "unknown")
	m.width = 100
	m.filtered = []model.Session{
		{
			ID:           "1",
			Agent:        model.AgentClaude,
			Project:      "myproject",
			State:        model.StateError,
			Model:        "claude-sonnet-4-6",
			Provider:     "anthropic",
			Branch:       "main",
			CWD:          "/home/user/src/myproject",
			LastUserText: "[Request interrupted by user for tool use]",
			LastEvent:    "A very long event message that would normally wrap if the detail pane does not truncate its content correctly.",
			Source:       "/home/user/.claude/projects/-home-user-src-myproject/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.jsonl",
			LastEventAt:  time.Now().Add(-time.Minute),
		},
	}

	view := m.detailView()
	if got, want := lipgloss.Height(view), detailPanelOuterHeight(); got != want {
		t.Fatalf("detailView height = %d, want %d; view:\n%s", got, want, view)
	}
}

func TestFullViewUsesConfiguredHeight(t *testing.T) {
	m := NewModel(nil, nil, "dev", "unknown")
	m.width = 120
	m.height = 40
	m.snapshot = model.Snapshot{
		Sessions: []model.Session{
			{
				ID:           "1",
				Agent:        model.AgentClaude,
				Project:      "myproject",
				State:        model.StateError,
				Model:        "claude-sonnet-4-6",
				Provider:     "anthropic",
				Branch:       "main",
				CWD:          "/home/user/src/myproject",
				Origin:       model.OriginMerged,
				LastUserText: "[Request interrupted by user for tool use]",
				LastEvent:    "A very long event message that would normally wrap if the detail pane does not truncate its content correctly.",
				Source:       "/home/user/.claude/projects/-home-user-src-myproject/aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee.jsonl",
				LastEventAt:  time.Now().Add(-time.Minute),
			},
		},
		CollectedAt: time.Now(),
	}
	m.applyRows()
	m.applyLayout()

	view := m.View()
	if got, want := lipgloss.Height(view), m.height; got != want {
		t.Fatalf("View height = %d, want %d; view:\n%s", got, want, view)
	}
}
