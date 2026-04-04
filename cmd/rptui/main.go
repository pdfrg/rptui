package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"rptui-bubbletea/internal/api"
	"rptui-bubbletea/internal/config"
	_ "rptui-bubbletea/internal/loginit"
	"rptui-bubbletea/internal/tui"
)

func main() {
	jukeboxMode := false
	args := os.Args[1:]

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--lastfm-auth":
			sessionKey, err := api.LastFMDoAuth()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			cfg, err := config.NewConfig()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
				os.Exit(1)
			}
			cfg.LastFM.SessionKey = sessionKey
			cfg.LastFM.Enabled = true
			if err := cfg.Save(); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("Session key saved to config. Last.fm scrobbling is now enabled.")
			return
		case "--jukebox", "-j":
			jukeboxMode = true
		}
	}

	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	theme, err := config.LoadTheme(cfg.ColorsFile, cfg.Theme)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load theme: %v\n", err)
		theme = config.DefaultTheme()
	}

	m := tui.NewModel(cfg, theme, jukeboxMode)

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Radio Paradise programming is human-curated and commercial-free.")
	fmt.Fprintln(os.Stderr, "Please consider supporting RP by visiting their website:")
	fmt.Fprintln(os.Stderr, "https://radioparadise.com/donate")
}
