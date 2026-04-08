package collect

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/juanpabloaj/workpulse/internal/model"
)

func homePath(elem ...string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	parts := append([]string{home}, elem...)
	return filepath.Join(parts...)
}

func tailLines(path string, maxLines int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > maxLines {
			lines = lines[1:]
		}
	}
	return lines, scanner.Err()
}

func firstLine(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if idx := bytes.IndexByte(data, '\n'); idx >= 0 {
		return string(data[:idx]), nil
	}
	return string(data), nil
}

func listRecentFiles(root, suffix string, limit int) ([]string, error) {
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
		if suffix != "" && !strings.HasSuffix(path, suffix) {
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

func shortenPath(path string) string {
	if path == "" {
		return "-"
	}
	base := filepath.Base(path)
	if base == "." || base == string(filepath.Separator) {
		return path
	}
	return base
}

func containsAnyLower(s string, needles ...string) bool {
	lower := strings.ToLower(s)
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func parseUnixMillis(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func trimForDisplay(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 96 {
		return s[:93] + "..."
	}
	return s
}

func parseTimeBestEffort(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts
	}
	return time.Time{}
}

func processMatchesAgent(proc model.ProcessInfo, agent model.AgentKind) bool {
	text := strings.ToLower(proc.Command + " " + proc.Args)
	switch agent {
	case model.AgentClaude:
		return strings.Contains(text, "claude")
	case model.AgentCodex:
		return strings.Contains(text, "codex")
	case model.AgentGemini:
		return strings.Contains(text, "gemini")
	default:
		return false
	}
}
