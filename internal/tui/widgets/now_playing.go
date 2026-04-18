// Package widgets provides reusable TUI components
package widgets

import (
	"fmt"
	"image/color"
	"log"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/pdfrg/rptui/internal/loginit"
	"github.com/pdfrg/rptui/internal/models"
)

var widgetLogger *log.Logger

func init() {
	widgetLogger = loginit.InitLogger("[NowPlaying] ")
}

// darkenColor reduces the brightness of a hex color by the given factor (0.0-1.0)
func darkenColor(hex string, factor float64) string {
	if hex == "default" || len(hex) != 7 || hex[0] != '#' {
		return hex
	}
	var r, g, b int
	fmt.Sscanf(hex[1:3], "%x", &r)
	fmt.Sscanf(hex[3:5], "%x", &g)
	fmt.Sscanf(hex[5:7], "%x", &b)
	r = int(float64(r) * (1 - factor))
	g = int(float64(g) * (1 - factor))
	b = int(float64(b) * (1 - factor))
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// lightenColor increases the brightness of a hex color by the given factor (0.0-1.0)
func lightenColor(hex string, factor float64) string {
	if len(hex) != 7 || hex[0] != '#' {
		return hex
	}
	var r, g, b int
	fmt.Sscanf(hex[1:3], "%x", &r)
	fmt.Sscanf(hex[3:5], "%x", &g)
	fmt.Sscanf(hex[5:7], "%x", &b)
	r = min(255, int(float64(r)*(1+factor)))
	g = min(255, int(float64(g)*(1+factor)))
	b = min(255, int(float64(b)*(1+factor)))
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// NowPlaying represents the current song info widget
type NowPlaying struct {
	foregroundStyle lipgloss.Style
	accentStyle     lipgloss.Style
	mutedStyle      lipgloss.Style
	cursorStyle     lipgloss.Style
	width           int
	maxWidth        int // when > 0, truncate title/artist/album with ellipsis
	contentWidth    int // when > 0, pad all lines to this exact width to prevent "clear to end of line"

	// Sleep timer display
	sleepTimerActive bool
	sleepTimerMins   int // remaining minutes

	// Bubbles progress bar
	progress progress.Model
}

// NewNowPlaying creates a new NowPlaying widget
func NewNowPlaying(fgStyle, accentStyle, mutedStyle lipgloss.Style, accentColor, cursorColor, progressBgColor string) *NowPlaying {
	cursorStyle := fgStyle
	if cursorColor != "" {
		cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(cursorColor))
	}

	var emptyColor color.Color
	// Progress bar always gets a proper shaded background
	// This is the ONLY place we ever draw an explicit background in transparent modes
	if progressBgColor != "" && len(progressBgColor) == 7 && progressBgColor[0] == '#' {
		emptyColor = lipgloss.Color(darkenColor(progressBgColor, 0.3))
	} else {
		// Neutral fallback
		emptyColor = lipgloss.Color("#1a1a1a")
	}

	p := progress.New(
		progress.WithWidth(40),
		progress.WithColors(lipgloss.Color(cursorColor), lipgloss.Color(accentColor)),
		progress.WithoutPercentage(),
		progress.WithFillCharacters('', ''),
	)
	p.EmptyColor = emptyColor

	return &NowPlaying{
		foregroundStyle: fgStyle,
		accentStyle:     accentStyle,
		mutedStyle:      mutedStyle,
		cursorStyle:     cursorStyle,
		progress:        p,
	}
}

// SetWidth sets the width of the widget
func (n *NowPlaying) SetWidth(width int) {
	n.width = width
	// Cap progress bar at 40 chars
	progWidth := min(40, width-2)
	progWidth = max(20, progWidth)
	n.progress.SetWidth(progWidth)
}

// GetWidth returns the current width of the widget
func (n *NowPlaying) GetWidth() int {
	return n.width
}

// SetMaxWidth sets the maximum width for text fields (title/artist/album).
// When > 0, text is truncated with ellipsis to prevent wrapping.
func (n *NowPlaying) SetMaxWidth(maxWidth int) {
	n.maxWidth = maxWidth
}

// SetContentWidth sets the exact width for all output lines.
// This prevents the renderer's "clear to end of line" from extending
// past this width, which would slice through album art rendered via tea.Raw().
// Only used for Large/Medium layouts where album art is on the right side.
func (n *NowPlaying) SetContentWidth(width int) {
	n.contentWidth = width
}

// UpdateStyles updates the widget styles with new theme colors
func (n *NowPlaying) UpdateStyles(fgStyle, accentStyle, mutedStyle lipgloss.Style, accentColor, cursorColor, bgColor string) {
	n.foregroundStyle = fgStyle
	n.accentStyle = accentStyle
	n.mutedStyle = mutedStyle

	n.cursorStyle = fgStyle
	if cursorColor != "" {
		n.cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(cursorColor))
	}

	var emptyColor color.Color
	if bgColor != "" && len(bgColor) == 7 && bgColor[0] == '#' {
		emptyColor = lipgloss.Color(darkenColor(bgColor, 0.3))
	}

	n.progress = progress.New(
		progress.WithWidth(40),
		progress.WithColors(lipgloss.Color(cursorColor), lipgloss.Color(accentColor)),
		progress.WithoutPercentage(),
		progress.WithFillCharacters('', ''),
	)
	n.progress.EmptyColor = emptyColor
}

// UpdateProgress updates the progress bar percent (0.0 to 1.0)
func (n *NowPlaying) UpdateProgress(percent float64) tea.Cmd {
	return n.progress.SetPercent(percent)
}

// Update handles messages (like FrameMsg for animations)
func (n *NowPlaying) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	n.progress, cmd = n.progress.Update(msg)
	return cmd
}

// SetSleepTimer sets the sleep timer display state
func (n *NowPlaying) SetSleepTimer(active bool, mins int) {
	n.sleepTimerActive = active
	n.sleepTimerMins = mins
}

// View renders the now playing info as a string
func (n NowPlaying) View(
	song *models.Song,
	isPaused bool,
	timePos float64,
	connectedTime time.Time,
	configInfo string,
	favoriteCount int,
	isFavorite bool,
	skipsAvailable int,
	prevAvailable int,
	statusMsg string,
	statusIsError bool,
	connErrorMsg string,
	minFavorites int,
	favoritesRemaining int,
	jukeboxMode bool,
	jukeboxPlayed int,
	jukeboxTotal int,
	offlineMode bool,
	offlineCacheInfo string,
	userRating string,
	rpFavIndicator string,
	cursorColor string,
) string {
	if song == nil {
		return " No song playing"
	}

	titleText := song.Title
	artistText := song.Artist
	albumText := fmt.Sprintf("%s (%s)", song.Album, song.Year)
	if n.maxWidth > 0 {
		titleText = ansi.Truncate(titleText, n.maxWidth, "...")
		artistText = ansi.Truncate(artistText, n.maxWidth, "...")
		albumText = ansi.Truncate(albumText, n.maxWidth, "...")
	}

	title := n.accentStyle.Bold(true).Render(titleText)
	artist := n.foregroundStyle.Render(artistText)
	album := n.mutedStyle.Render(albumText)

	progView := n.progress.View()

	percentPos := timePos / song.GetDurationSeconds() * 100
	if song.GetDurationSeconds() <= 0 {
		percentPos = 0
	}

	timeStr := n.mutedStyle.Render(fmt.Sprintf("%s / %s (%.0f%%)",
		formatDuration(timePos),
		formatDuration(song.GetDurationSeconds()),
		percentPos))

	rating := song.Rating
	if rating != "" && rating != "—" {
		var avg float64
		fmt.Sscanf(rating, "%f", &avg)
		if avg > 0 {
			rating = fmt.Sprintf("%.1f", avg)
		}
	}
	if isFavorite {
		rating = "★ " + rating
	}

	var ratingLine string
	if userRating != "" {
		keyStyle := n.mutedStyle.Render("│")
		keyEmoji := n.cursorStyle.Copy().Render("🔑")
		displayRating := userRating
		if userRating == "0" {
			displayRating = "--"
		}
		userRatingStyle := n.cursorStyle.Copy().Render(displayRating)
		ratingLine = fmt.Sprintf("%s  %s  %s %s", n.accentStyle.Render(rating), keyStyle, keyEmoji, userRatingStyle)
	} else {
		ratingLine = n.accentStyle.Render(rating)
	}

	// Append RP favorites indicator if present
	if rpFavIndicator != "" {
		ratingLine = fmt.Sprintf("%s  %s", ratingLine, rpFavIndicator)
	}
	ratingStr := ratingLine

	// Navigation line
	nextStr := "--"
	if skipsAvailable > 0 {
		nextStr = fmt.Sprintf("%d", skipsAvailable)
	}
	prevStr := "--"
	if prevAvailable > 0 {
		prevStr = fmt.Sprintf("%d", prevAvailable)
	}

	var navLine string
	if jukeboxMode {
		jukeStr := fmt.Sprintf("%s %s/%s",
			n.foregroundStyle.Render("🎶"),
			n.foregroundStyle.Render(fmt.Sprintf("%d", jukeboxPlayed)),
			n.mutedStyle.Render(fmt.Sprintf("%d", jukeboxTotal)))
		navLine = fmt.Sprintf("%s %s  %s %s  %s",
			n.mutedStyle.Render("󰒮"), n.foregroundStyle.Render(prevStr),
			n.mutedStyle.Render("󰒭"), n.foregroundStyle.Render(nextStr),
			jukeStr)
	} else if offlineMode {
		favStr := fmt.Sprintf("%s%s%s",
			n.foregroundStyle.Render(fmt.Sprintf("%d", favoriteCount)),
			n.mutedStyle.Render("/"),
			n.mutedStyle.Render(fmt.Sprintf("%d", minFavorites)))
		if favoriteCount >= minFavorites {
			remaining := favoritesRemaining
			if remaining == 0 {
				remaining = favoriteCount
			}
			remainingStr := fmt.Sprintf("%s%s%s",
				n.mutedStyle.Render("<"),
				n.foregroundStyle.Render(fmt.Sprintf("%d", remaining)),
				n.mutedStyle.Render(">"))
			favStr += " ✅ " + remainingStr
		}
		navLine = fmt.Sprintf("%s %s  %s %s  ⭐ %s",
			n.mutedStyle.Render("󰒮"), n.foregroundStyle.Render(prevStr),
			n.mutedStyle.Render("󰒭"), n.foregroundStyle.Render(nextStr),
			favStr)
	} else {
		favStr := fmt.Sprintf("%s%s%s",
			n.foregroundStyle.Render(fmt.Sprintf("%d", favoriteCount)),
			n.mutedStyle.Render("/"),
			n.mutedStyle.Render(fmt.Sprintf("%d", minFavorites)))
		if favoriteCount >= minFavorites {
			remaining := favoritesRemaining
			if remaining == 0 {
				remaining = favoriteCount
			}
			remainingStr := fmt.Sprintf("%s%s%s",
				n.mutedStyle.Render("<"),
				n.foregroundStyle.Render(fmt.Sprintf("%d", remaining)),
				n.mutedStyle.Render(">"))
			favStr += " ✅ " + remainingStr
		}
		navLine = fmt.Sprintf("%s %s  %s %s  ⭐ %s",
			n.mutedStyle.Render("󰒮"), n.foregroundStyle.Render(prevStr),
			n.mutedStyle.Render("󰒭"), n.foregroundStyle.Render(nextStr),
			favStr)
	}

	var statusLine string
	if offlineMode {
		status := "Playing"
		if isPaused {
			status = n.mutedStyle.Render("Paused")
		} else {
			status = n.mutedStyle.Render("Playing")
		}
		statusLine = fmt.Sprintf("%s %s %s", status, n.mutedStyle.Render("•"), n.mutedStyle.Render(offlineCacheInfo))
	} else if jukeboxMode {
		status := "Playing"
		if isPaused {
			status = n.mutedStyle.Render("Paused")
		} else {
			status = n.mutedStyle.Render("Playing")
		}
		statusLine = status
	} else {
		status := "Playing"
		if isPaused {
			status = n.mutedStyle.Render("Paused")
		} else {
			status = n.mutedStyle.Render("Playing")
		}
		statusLine = fmt.Sprintf("%s %s %s", status, n.mutedStyle.Render("•"), n.mutedStyle.Render(configInfo))
	}

	var connectedLine string
	if offlineMode {
		if connErrorMsg != "" {
			connectedLine = n.accentStyle.Render(connErrorMsg)
		} else if statusMsg != "" {
			if statusIsError {
				connectedLine = n.accentStyle.Render(statusMsg)
			} else {
				connectedLine = n.foregroundStyle.Render(statusMsg)
			}
		} else {
			connectedLine = n.mutedStyle.Render(fmt.Sprintf("Offline Mode • %s", connectedTime.Format("15:04:05")))
		}
	} else if jukeboxMode {
		if connErrorMsg != "" {
			connectedLine = n.accentStyle.Render(connErrorMsg)
		} else if statusMsg != "" {
			if statusIsError {
				connectedLine = n.accentStyle.Render(statusMsg)
			} else {
				connectedLine = n.foregroundStyle.Render(statusMsg)
			}
		} else {
			connectedLine = n.mutedStyle.Render(fmt.Sprintf("Jukebox mode • %s", connectedTime.Format("15:04:05")))
		}
	} else {
		if connErrorMsg != "" {
			connectedLine = n.accentStyle.Render(connErrorMsg)
		} else if statusMsg != "" {
			if statusIsError {
				connectedLine = n.accentStyle.Render(statusMsg)
			} else {
				connectedLine = n.foregroundStyle.Render(statusMsg)
			}
		} else {
			connectedLine = n.mutedStyle.Render(fmt.Sprintf("Connected • %s", connectedTime.Format("15:04:05")))
		}
	}

	// Append sleep timer info if active
	if n.sleepTimerActive {
		connectedLine = fmt.Sprintf("%s %s %s", connectedLine, n.mutedStyle.Render("•"), n.accentStyle.Render(fmt.Sprintf("Sleep in %dm", n.sleepTimerMins)))
	}

	// Build output
	output := fmt.Sprintf(" %s\n %s\n %s\n\n %s\n %s\n\n %s\n\n %s\n\n %s\n\n %s\n\n",
		title, artist, album, progView, timeStr, ratingStr, navLine, statusLine, connectedLine)

	// Apply width limit if set - this prevents the renderer's "clear to end of line"
	// from extending past this width and slicing through album art rendered via tea.Raw()
	if n.contentWidth > 0 {
		lines := strings.Split(output, "\n")
		for i, line := range lines {
			originalWidth := lipgloss.Width(line)
			if originalWidth > n.contentWidth {
				// Truncate with ellipsis if too long
				line = ansi.Truncate(line, n.contentWidth-3, "...")
			}
			// Pad to exact width (lipgloss.Width adds trailing spaces)
			if lipgloss.Width(line) < n.contentWidth {
				line = lipgloss.NewStyle().Width(n.contentWidth).Render(line)
			}
			lines[i] = line
		}
		output = strings.Join(lines, "\n")
	}

	return output
}

// formatDuration formats seconds as MM:SS or HH:MM:SS
func formatDuration(seconds float64) string {
	totalSeconds := int(seconds)
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	secs := totalSeconds % 60

	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}
