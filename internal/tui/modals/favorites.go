// Package modals provides modal dialogs for the TUI
package modals

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rptui-bubbletea/internal/cache"
	"rptui-bubbletea/internal/config"
)

// FavoritesMsg is sent when the favorites modal closes
type FavoritesMsg struct {
	PlayEventID *int64 // If set, play this favorite
	Closed      bool
}

// Tab constants for favorites modal
const (
	TabFavorites = iota
	TabBlocklist
)

// Favorites modal for managing favorites and blocklist
type Favorites struct {
	styles       *config.ThemeStyles
	cacheManager *cache.CacheManager
	activeTab    int
	cursor       int
	scrollOffset int
	favorites    []cache.CachedSong
	blocklist    []cache.CachedSong
	termWidth    int
	termHeight   int
}

// NewFavorites creates a new Favorites modal
func NewFavorites(styles *config.ThemeStyles, cacheManager *cache.CacheManager, termWidth, termHeight int) *Favorites {
	f := &Favorites{
		styles:       styles,
		cacheManager: cacheManager,
		termWidth:    termWidth,
		termHeight:   termHeight,
	}
	f.loadData()
	return f
}

func (f *Favorites) loadData() {
	if favs, err := f.cacheManager.GetFavorites(); err == nil {
		f.favorites = favs
	}
	if blocks, err := f.cacheManager.GetBlocklist(); err == nil {
		f.blocklist = blocks
	}
}

func (f *Favorites) activeItems() []cache.CachedSong {
	if f.activeTab == TabFavorites {
		return f.favorites
	}
	return f.blocklist
}

func (f *Favorites) visibleRows() int {
	rows := f.termHeight - 18
	if rows < 5 {
		rows = 5
	}
	return rows
}

// Update handles messages
func (f *Favorites) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		items := f.activeItems()
		visible := f.visibleRows()

		switch msg.String() {
		case "esc", "q":
			return func() tea.Msg { return FavoritesMsg{Closed: true} }

		case "tab":
			f.activeTab = (f.activeTab + 1) % 2
			f.cursor = 0
			f.scrollOffset = 0
			return nil

		case "up", "k":
			if f.cursor > 0 {
				f.cursor--
				if f.cursor < f.scrollOffset {
					f.scrollOffset = f.cursor
				}
			}

		case "down", "j":
			if f.cursor < len(items)-1 {
				f.cursor++
				if f.cursor >= f.scrollOffset+visible {
					f.scrollOffset = f.cursor - visible + 1
				}
			}

		case "d", "delete", "x":
			if len(items) > 0 && f.cursor < len(items) {
				item := items[f.cursor]
				if f.activeTab == TabFavorites {
					f.cacheManager.RemoveFavorite(item.EventID)
				} else {
					f.cacheManager.RemoveBlocklist(item.EventID)
				}
				f.loadData()
				items = f.activeItems()
				if f.cursor >= len(items) && f.cursor > 0 {
					f.cursor--
				}
				if f.scrollOffset > 0 && f.scrollOffset >= len(items)-visible {
					f.scrollOffset = max(0, len(items)-visible)
				}
			}
			return nil

		case "enter":
			if f.activeTab == TabFavorites && len(items) > 0 && f.cursor < len(items) {
				eventID := items[f.cursor].EventID
				return func() tea.Msg {
					return FavoritesMsg{PlayEventID: &eventID}
				}
			}
			return nil
		}
	}
	return nil
}

// View renders the modal
func (f Favorites) View() string {
	// Use full terminal width with small margin
	modalWidth := f.termWidth - 4
	if modalWidth < 60 {
		modalWidth = 60
	}
	// border (2) + padding (2*2) = 6
	contentWidth := modalWidth - 6
	// prefix "▸ " = 2 chars
	rowWidth := contentWidth - 2

	// Dynamic column widths: Song 40%, Artist 30%, Album 30%
	songCol := rowWidth * 40 / 100
	artistCol := rowWidth * 30 / 100
	albumCol := rowWidth - songCol - artistCol

	// Dynamic list height from terminal height
	// overhead: border(2) + padding(2) + title(1) + blank(1) + tabs(1) + stats(1) + blank(1) + header(1) + sep(1) + blank(1) + scroll(1) + help(1) + centering margin
	visibleRows := f.termHeight - 18
	if visibleRows < 5 {
		visibleRows = 5
	}

	accentStyle := f.styles.AccentStyle
	mutedStyle := f.styles.MutedStyle
	cursorStyle := f.styles.CursorStyle

	var b strings.Builder

	// Title
	title := accentStyle.Render("MANAGE")
	b.WriteString(centerStyled(title, contentWidth))
	b.WriteString("\n\n")

	// Tabs
	favLabel := mutedStyle.Render("Favorites")
	blockLabel := mutedStyle.Render("Blocklist")
	if f.activeTab == TabFavorites {
		favLabel = cursorStyle.Render("▸ Favorites")
	} else {
		blockLabel = cursorStyle.Render("▸ Blocklist")
	}
	tabs := favLabel + mutedStyle.Render("  │  ") + blockLabel
	b.WriteString(centerStyled(tabs, contentWidth))
	b.WriteString("\n")

	// Stats line
	stats := mutedStyle.Render(fmt.Sprintf("%d favorites  %d blocklisted", len(f.favorites), len(f.blocklist)))
	b.WriteString(centerStyled(stats, contentWidth))
	b.WriteString("\n\n")

	// Column headers
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(f.styles.Muted)).
		Bold(true)
	header := headerStyle.Render(fmt.Sprintf("  %-*s %-*s %-*s", songCol, "Song", artistCol, "Artist", albumCol, "Album"))
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(strings.Repeat("─", contentWidth)))
	b.WriteString("\n")

	// List items
	items := f.activeItems()
	if len(items) == 0 {
		emptyMsg := "No favorites yet. Press f to add favorites."
		if f.activeTab == TabBlocklist {
			emptyMsg = "No blocklisted songs. Press b to blocklist."
		}
		b.WriteString("\n")
		b.WriteString(centerStyled(mutedStyle.Render(emptyMsg), contentWidth))
		b.WriteString("\n")
		for i := 0; i < visibleRows-2; i++ {
			b.WriteString("\n")
		}
	} else {
		end := f.scrollOffset + visibleRows
		if end > len(items) {
			end = len(items)
		}
		for i := f.scrollOffset; i < end; i++ {
			item := items[i]
			song := truncate(item.Title, songCol-2)
			artist := truncate(item.Artist, artistCol-2)
			album := truncate(item.Album, albumCol-2)
			row := fmt.Sprintf("%-*s %-*s %-*s", songCol, song, artistCol, artist, albumCol, album)

			if i == f.cursor {
				prefix := cursorStyle.Render("▸ ")
				line := lipgloss.NewStyle().
					Foreground(lipgloss.Color(f.styles.Foreground)).
					Render(row)
				b.WriteString(prefix + line)
			} else {
				b.WriteString("  " + mutedStyle.Render(row))
			}
			b.WriteString("\n")
		}
		for i := end - f.scrollOffset; i < visibleRows; i++ {
			b.WriteString("\n")
		}
	}

	// Scroll indicator
	if len(items) > visibleRows {
		scrollInfo := mutedStyle.Render(fmt.Sprintf("  %d-%d of %d", f.scrollOffset+1, min(f.scrollOffset+visibleRows, len(items)), len(items)))
		b.WriteString(scrollInfo)
	}
	b.WriteString("\n\n")

	// Help
	helpParts := []string{
		accentStyle.Render("↑/↓") + mutedStyle.Render(" navigate"),
		accentStyle.Render("d") + mutedStyle.Render(" delete"),
	}
	if f.activeTab == TabFavorites {
		helpParts = append(helpParts, accentStyle.Render("enter")+mutedStyle.Render(" play"))
	}
	helpParts = append(helpParts,
		accentStyle.Render("tab")+mutedStyle.Render(" switch"),
		accentStyle.Render("esc")+mutedStyle.Render(" close"),
	)
	helpText := strings.Join(helpParts, mutedStyle.Render("  "))
	b.WriteString(centerStyled(helpText, contentWidth))

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(f.styles.Accent)).
		Padding(1, 2).
		Width(modalWidth)

	return modalStyle.Render(b.String())
}

// truncate truncates a string to maxLen, adding ellipsis if needed
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}
