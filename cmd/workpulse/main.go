package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/juanpabloaj/workpulse/internal/collect"
	"github.com/juanpabloaj/workpulse/internal/tui"
)

var version = "dev"
var buildDate = "unknown"

func main() {
	version, buildDate = resolveBuildMetadata(version, buildDate)
	if hasVersionFlag(os.Args[1:]) {
		fmt.Println(formatVersion(version, buildDate))
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	collector := collect.NewCollector()
	model := tui.NewModel(ctx, collector, version, buildDate)

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "workpulse: %v\n", err)
		os.Exit(1)
	}
}

func hasVersionFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--version" {
			return true
		}
	}
	return false
}

func formatVersion(version, buildDate string) string {
	return fmt.Sprintf("workpulse %s (built %s)", version, buildDate)
}

func resolveBuildMetadata(currentVersion, currentBuildDate string) (string, string) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return currentVersion, currentBuildDate
	}

	if currentVersion == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		currentVersion = info.Main.Version
	}
	if currentBuildDate == "unknown" {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.time" && setting.Value != "" {
				currentBuildDate = setting.Value
				break
			}
		}
	}

	return currentVersion, currentBuildDate
}
