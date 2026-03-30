// Package tui provides the terminal user interface
package tui

import (
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"
	"rptui-bubbletea/internal/api"
	"rptui-bubbletea/internal/config"
	"rptui-bubbletea/internal/models"
)

// Custom message types for bubbletea

// blockFetchedMsg is sent when a block is fetched from API
type blockFetchedMsg struct {
	songs     []*models.Song
	imageBase string
	blockID   int // for detecting new vs cached response
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

// artistStatusMsg is sent during async artist fetch to report progress
type artistStatusMsg struct {
	eventID int64
	status  string // "Searching Discogs...", "Fetching discography...", etc.
}

// artistFetchedMsg is sent when artist info is fetched
type artistFetchedMsg struct {
	eventID int64
	info    *models.ArtistInfo
	err     error
}

// imageLoadedMsg is sent when album art image is loaded
type imageLoadedMsg struct {
	eventID   int64
	imageData []byte
	err       error
}

// artistImageLoadedMsg is sent when artist thumbnail image is loaded
type artistImageLoadedMsg struct {
	eventID   int64
	imageData []byte
	err       error
}

// renderArtistArtMsg is sent after a short delay to re-render artist thumbnail
type renderArtistArtMsg struct{}

// statusClearMsg is sent after a timeout to clear temporary status messages
type statusClearMsg struct {
	seq int
}

// connRetryTickMsg is sent on the next retry interval during backoff
type connRetryTickMsg time.Time

// renderAlbumArtMsg is sent after a short delay to re-render album art.
// The delay ensures the cell-based renderer has finished its redraw before
// we send the Kitty graphics escape sequence via tea.Raw.
type renderAlbumArtMsg struct{}

// themeChangedMsg is sent when the theme file is modified on disk
type themeChangedMsg struct {
	path string
}

// Command functions

// renderAlbumArtAfterDelay returns a command that triggers album art re-render
// after a short delay, allowing the cell renderer to finish its redraw first.
func renderAlbumArtAfterDelay() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return renderAlbumArtMsg{}
	})
}

// renderArtistArtAfterDelay returns a command that triggers artist thumbnail re-render
// after a short delay, allowing the cell renderer to finish its redraw first.
func renderArtistArtAfterDelay() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return renderArtistArtMsg{}
	})
}

// clearKittyImagesCmd sends the Kitty graphics clear escape sequence directly
// to the terminal via tea.Raw, bypassing the cell-based renderer. This is
// needed because APC sequences embedded in View() content get consumed by the
// cell buffer and don't reliably reach the terminal.
func clearKittyImagesCmd() tea.Cmd {
	return tea.Raw("\x1b_Ga=d,d=A\x1b\\")
}

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

// setStatus sets a temporary status message that auto-clears after 5 seconds.
// Returns a tea.Cmd (possibly nil) that should be batched with other commands.
func setStatus(m *Model, msg string, isError bool) tea.Cmd {
	m.statusMsg = msg
	m.statusIsError = isError
	m.statusSeq++
	seq := m.statusSeq
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return statusClearMsg{seq: seq}
	})
}

// tickConnRetryCmd returns a command that fires after the given duration (retry backoff)
func tickConnRetryCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return connRetryTickMsg(t)
	})
}

// watchThemeCmd returns a command that listens for theme file changes
func watchThemeCmd(watcher *config.ThemeWatcher) tea.Cmd {
	return func() tea.Msg {
		path := <-watcher.Events()
		return themeChangedMsg{path: path}
	}
}
