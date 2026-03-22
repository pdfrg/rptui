// Package tui provides the terminal user interface
package tui

import (
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"rptui-bubbletea/internal/api"
	"rptui-bubbletea/internal/models"
)

// Custom message types for bubbletea

// blockFetchedMsg is sent when a block is fetched from API
type blockFetchedMsg struct {
	songs     []*models.Song
	imageBase string
	err       error
}

// progressTickMsg is sent every second to update progress
type progressTickMsg time.Time

// pollTickMsg is sent every 5 seconds to poll for next block
type pollTickMsg time.Time

// lyricsFetchedMsg is sent when lyrics are fetched
type lyricsFetchedMsg struct {
	eventID int64
	plain   string
	synced  []api.SyncedLyric
	err     error
}

// artistFetchedMsg is sent when artist info is fetched
type artistFetchedMsg struct {
	eventID int64
	info    *api.ArtistInfo
	err     error
}

// imageLoadedMsg is sent when album art image is loaded
type imageLoadedMsg struct {
	eventID   int64
	imageData []byte
	err       error
}

// Command functions

// tickProgressCmd returns a command that sends progressTickMsg every second
func tickProgressCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return progressTickMsg(t)
	})
}

// tickPollCmd returns a command that sends pollTickMsg every 5 seconds
func tickPollCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return pollTickMsg(t)
	})
}

// openDonatePageCmd opens the Radio Paradise donate page in the default browser
func openDonatePageCmd() tea.Msg {
	cmd := exec.Command("xdg-open", "https://radioparadise.com/donate")
	cmd.Start()
	return nil
}
