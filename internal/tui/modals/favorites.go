// Package modals provides modal dialogs for the TUI
package modals

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/table"
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
	width        int
	height       int
	activeTab    int // TabFavorites or TabBlocklist
	table        table.Model
	favorites    []cache.CachedSong
	blocklist    []cache.CachedSong
}

// NewFavorites creates a new Favorites modal
func NewFavorites(styles *config.ThemeStyles, cacheManager *cache.CacheManager) *Favorites {
	columns := []table.Column{
		{Title: "#", Width: 3},
		{Title: "Song", Width: 30},
		{Title: "Artist", Width: 20},
		{Title: "Album", Width: 20},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(10),
		table.WithWidth(76),
	)

	// Apply styles
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(styles.Muted)).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color(styles.Cursor)).
		Bold(true)
	t.SetStyles(s)

	f := &Favorites{
		styles:       styles,
		cacheManager: cacheManager,
		table:        t,
	}
	f.loadFavorites()
	f.loadBlocklist()
	return f
}

// loadFavorites loads favorites from cache
func (f *Favorites) loadFavorites() {
	favorites, err := f.cacheManager.GetFavorites()
	if err != nil {
		favorites = nil
	}
	f.favorites = favorites
	f.updateTable()
}

// loadBlocklist loads blocklist from cache
func (f *Favorites) loadBlocklist() {
	blocklist, err := f.cacheManager.GetBlocklist()
	if err != nil {
		blocklist = nil
	}
	f.blocklist = blocklist
	f.updateTable()
}

// updateTable updates the table rows based on active tab
func (f *Favorites) updateTable() {
	var items []cache.CachedSong
	if f.activeTab == TabFavorites {
		items = f.favorites
	} else {
		items = f.blocklist
	}

	rows := make([]table.Row, len(items))
	for i, item := range items {
		rows[i] = table.Row{
			fmt.Sprintf("%d", i+1),
			item.Title,
			item.Artist,
			item.Album,
		}
	}

	f.table.SetRows(rows)
	if len(rows) > 0 {
		f.table.SetCursor(0)
	}
}

// Update handles messages
func (f *Favorites) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "q":
			return func() tea.Msg { return FavoritesMsg{Closed: true} }

		case "tab":
			// Switch between favorites and blocklist
			f.activeTab = (f.activeTab + 1) % 2
			if f.activeTab == TabFavorites {
				f.loadFavorites()
			} else {
				f.loadBlocklist()
			}
			return nil

		case "d", "delete":
			// Delete selected item
			cursor := f.table.Cursor()
			if f.activeTab == TabFavorites && cursor < len(f.favorites) {
				fav := f.favorites[cursor]
				f.cacheManager.RemoveFavoriteByID(fav.EventID)
				f.loadFavorites()
			} else if f.activeTab == TabBlocklist && cursor < len(f.blocklist) {
				item := f.blocklist[cursor]
				f.cacheManager.RemoveBlocklistByID(item.EventID)
				f.loadBlocklist()
			}
			return nil

		case "enter":
			// Play selected favorite
			if f.activeTab == TabFavorites {
				cursor := f.table.Cursor()
				if cursor < len(f.favorites) {
					eventID := f.favorites[cursor].EventID
					return func() tea.Msg {
						return FavoritesMsg{PlayEventID: &eventID}
					}
				}
			}
			return nil

		default:
			// Forward to table for navigation
			var cmd tea.Cmd
			f.table, cmd = f.table.Update(msg)
			return cmd
		}

	default:
		var cmd tea.Cmd
		f.table, cmd = f.table.Update(msg)
		return cmd
	}
}

// centerText centers plain (unstyled) text within a given width
func centerText(text string, width int) string {
	if len(text) >= width {
		return text
	}
	padding := (width - len(text)) / 2
	return strings.Repeat(" ", padding) + text
}

// View renders the modal
func (f Favorites) View() string {
	modalWidth := 80
	modalHeight := 20

	var b strings.Builder

	// Header
	tabFavorites := "Favorites"
	tabBlocklist := "Blocklist"
	if f.activeTab == TabFavorites {
		tabFavorites = f.styles.AccentStyle.Render("▸ Favorites")
	} else {
		tabBlocklist = f.styles.AccentStyle.Render("▸ Blocklist")
	}
	b.WriteString(centerText(fmt.Sprintf("[ %s | %s ]", tabFavorites, tabBlocklist), modalWidth))
	b.WriteString("\n\n")

	// Stats
	favCount := len(f.favorites)
	blockCount := len(f.blocklist)
	stats := f.styles.MutedStyle.Render(fmt.Sprintf("%d favorites | %d blocklisted", favCount, blockCount))
	b.WriteString(centerText(stats, modalWidth))
	b.WriteString("\n\n")

	// Table
	b.WriteString(f.table.View())
	b.WriteString("\n\n")

	// Help
	help := f.styles.MutedStyle.Render("↑/↓:Navigate  Enter:Play  d:Delete  Tab:Switch  Esc:Close")
	b.WriteString(centerText(help, modalWidth))

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(f.styles.Accent)).
		Padding(1, 2).
		Width(modalWidth).
		Height(modalHeight)

	return modalStyle.Render(b.String())
}
