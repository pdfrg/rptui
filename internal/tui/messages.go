// Package tui provides the terminal user interface
package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/pdfrg/rptui/internal/api"
	"github.com/pdfrg/rptui/internal/cache"
	"github.com/pdfrg/rptui/internal/config"
	"github.com/pdfrg/rptui/internal/models"
)

// Custom message types for bubbletea

// blockFetchedMsg is sent when a block is fetched from API
type blockFetchedMsg struct {
	songs     []*models.Song
	imageBase string
	blockID   int // for detecting new vs cached response
	err       error
}

// chan99FetchedMsg is sent when channel 99 songs are fetched
type chan99FetchedMsg struct {
	songs []*models.Song
	count int // how many were requested
	err   error
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

// commentsFetchedMsg is sent when comments are fetched
type commentsFetchedMsg struct {
	songID   int64
	comments []*api.Comment
	total    int
	more     bool
	offset   int
	err      error
	loadMore bool // true when loading additional pages (not initial fetch)
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

// scrobbleResultMsg is sent when a scrobble attempt completes with results
type scrobbleResultMsg struct {
	results []api.ScrobbleResult
}

// visTickMsg is sent every 50ms to update the visualizer animation
type visTickMsg time.Time

// notificationSentMsg is sent when a desktop notification has been dispatched
type notificationSentMsg struct{}

// favoriteDownloadMsg is sent when a favorite audio download completes
type favoriteDownloadMsg struct {
	success bool
}

// sleepTimerTickMsg is sent every minute to update sleep timer display
type sleepTimerTickMsg time.Time

// quitTickMsg is sent every second during quitting countdown
type quitTickMsg time.Time

// tickSleepTimerCmd returns a command that sends sleepTimerTickMsg every minute
func tickSleepTimerCmd() tea.Cmd {
	return tea.Tick(time.Minute, func(t time.Time) tea.Msg {
		return sleepTimerTickMsg(t)
	})
}

// tickQuitCmd returns a command that sends quitTickMsg every second during quitting countdown
func tickQuitCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return quitTickMsg(t)
	})
}

// jukeboxStartMsg is sent to trigger jukebox playback initialization
type jukeboxStartMsg struct{}

// offlineStartMsg is sent to trigger offline playback initialization
type offlineStartMsg struct{}

// stationCheckResultMsg is sent when station validation completes
type stationCheckResultMsg struct {
	issues []config.StationIssue
}

// Command functions

// checkStationsCmd runs station validation in the background
func (m Model) checkStationsCmd() tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rpChannels, err := m.rpAPI.ListChannels(ctx)
	if err != nil {
		logger.Printf("Station validation failed: %v", err)
		return stationCheckResultMsg{}
	}

	// Build map of downloadable channels
	chMap := make(map[int]string)
	for _, ch := range rpChannels {
		if ch.Downloadable {
			var id int
			fmt.Sscanf(ch.Chan, "%d", &id)
			chMap[id] = ch.Title
		}
	}

	issues := config.CheckStationIssues(chMap)
	return stationCheckResultMsg{issues: issues}
}

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

// tickVisCmd returns a command that sends visTickMsg every 50ms (20 FPS)
func tickVisCmd() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(t time.Time) tea.Msg {
		return visTickMsg(t)
	})
}

// scrobbleCmd runs the scrobble and returns results as a message.
func scrobbleCmd(scrobbler *api.Scrobbler, song models.Song, startTime time.Time) tea.Cmd {
	return func() tea.Msg {
		results := scrobbler.ScrobbleWithResult(context.Background(), song, startTime)
		return scrobbleResultMsg{results: results}
	}
}

// copyToClipboardCmd copies formatted song info to the system clipboard.
// It tries wl-copy (Wayland), xclip/xsel (X11), then pbcopy (macOS) in order.
func copyToClipboardCmd(song *models.Song) tea.Cmd {
	return func() tea.Msg {
		info := song.FormatDisplayInfo()

		clipboardCmds := [][]string{
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
			{"pbcopy"},
		}

		for _, args := range clipboardCmds {
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Stdin = strings.NewReader(info)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}

		return nil
	}
}

// favoriteDownloadCmd starts a favorite download and returns a command
func favoriteDownloadCmd(cmgr *cache.CacheManager, song *models.Song, fileExt string, results chan<- favoriteDownloadMsg) tea.Cmd {
	return func() tea.Msg {
		cmgr.StartFavoriteDownload(song, fileExt, func(success bool) {
			results <- favoriteDownloadMsg{success: success}
		})
		return nil
	}
}

// startJukeboxCmd returns a command that triggers jukebox initialization
func startJukeboxCmd() tea.Cmd {
	return func() tea.Msg {
		return jukeboxStartMsg{}
	}
}

// startOfflineCmd returns a command that triggers offline initialization
func startOfflineCmd() tea.Cmd {
	return func() tea.Msg {
		return offlineStartMsg{}
	}
}

// fetchCommentsCmd fetches comments for the given song
func fetchCommentsCmd(client *api.RPCommentsClient, songID int64) tea.Cmd {
	return func() tea.Msg {
		if client == nil || songID == 0 {
			return commentsFetchedMsg{songID: songID, err: fmt.Errorf("no comments available")}
		}
		resp, err := client.GetComments(songID, 20, "oldest")
		if err != nil {
			return commentsFetchedMsg{songID: songID, err: err}
		}
		comments := make([]*api.Comment, len(resp.Comments))
		for i := range resp.Comments {
			comments[i] = &resp.Comments[i]
		}
		return commentsFetchedMsg{
			songID:   songID,
			comments: comments,
			total:    resp.TotalComments,
			more:     resp.MoreComments,
			offset:   resp.MoreOffset,
		}
	}
}

// fetchChan99Cmd fetches n songs from channel 99 by calling /play n times
func fetchChan99Cmd(rpAPI *api.RadioParadiseAPI, count int) tea.Cmd {
	return func() tea.Msg {
		var allSongs []*models.Song
		for i := 0; i < count; i++ {
			block, err := rpAPI.GetBlock(context.Background())
			if err != nil {
				return chan99FetchedMsg{count: count, err: fmt.Errorf("failed to fetch channel 99 song %d: %w", i+1, err)}
			}
			songs, _ := rpAPI.ParseBlockSongs(block)
			if len(songs) == 0 {
				continue
			}
			allSongs = append(allSongs, songs...)
		}
		if len(allSongs) == 0 {
			return chan99FetchedMsg{count: count, err: fmt.Errorf("no songs returned from channel 99")}
		}
		return chan99FetchedMsg{songs: allSongs, count: count}
	}
}
