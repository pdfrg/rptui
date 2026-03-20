package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"rptui-bubbletea/internal/config"
	"rptui-bubbletea/internal/tui"
)

func main() {
	// Load config
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Load theme
	theme, err := config.LoadTheme(cfg.ColorsFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load theme: %v\n", err)
		theme = config.DefaultTheme()
	}

	// Create TUI model
	m := tui.NewModel(cfg, theme)

	// Run program
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}
