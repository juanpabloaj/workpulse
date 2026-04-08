package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/juanpabloaj/workpulse/internal/collect"
	"github.com/juanpabloaj/workpulse/internal/tui"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	collector := collect.NewCollector()
	model := tui.NewModel(ctx, collector)

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "workpulse: %v\n", err)
		os.Exit(1)
	}
}
