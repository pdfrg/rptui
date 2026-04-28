// Package modals provides modal dialogs for the TUI
package modals

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/pdfrg/rptui/internal/cache"
	"github.com/pdfrg/rptui/internal/config"
)

// FavoritesMsg is sent when the favorites modal closes
type FavoritesMsg struct {
	PlayEventID    *int64
	EnqueueEventID *int64
	StayOpen       bool
	Closed         bool
}

// Tab constants for favorites modal
const (
	TabFavorites = iota
	TabBlocklist
	TabOffline
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
	offlineDir   string
	offlineList  []cache.CacheEntry
	termWidth    int
	termHeight   int
	searchInput  textinput.Model
	isSearching  bool
	toastMessage string
}

// NewFavorites creates a new Favorites modal
func NewFavorites(styles *config.ThemeStyles, cacheManager *cache.CacheManager, termWidth, termHeight int) *Favorites {
	f := &Favorites{
		styles:       styles,
		cacheManager: cacheManager,
		termWidth:    termWidth,
		termHeight:   termHeight,
		searchInput:  textinput.New(),
		offlineDir:   cacheManager.GetOfflineDir(),
	}
	f.searchInput.Placeholder = "Search favorites..."
	f.searchInput.CharLimit = 64
	f.searchInput.SetWidth(termWidth - 20)
	f.searchInput.Prompt = ""
	f.loadData()
	return f
}

func (f *Favorites) loadData() {
	if favs, err := f.cacheManager.GetFavorites(); err == nil {
		f.favorites = favs
		slices.SortFunc(f.favorites, func(a, b cache.CachedSong) int {
			return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		})
	}
	if blocks, err := f.cacheManager.GetBlocklist(); err == nil {
		f.blocklist = blocks
		slices.SortFunc(f.blocklist, func(a, b cache.CachedSong) int {
			return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		})
	}
	if f.offlineDir != "" {
		f.offlineList, _ = cache.ListCaches(f.offlineDir)
	}
}

func (f *Favorites) activeItems() []cache.CachedSong {
	if f.activeTab == TabFavorites {
		return f.favorites
	}
	return f.blocklist
}

func (f *Favorites) activeOfflineItems() []cache.CacheEntry {
	if f.activeTab == TabOffline {
		return f.offlineList
	}
	return nil
}

func (f *Favorites) filteredItems() []cache.CachedSong {
	items := f.activeItems()
	if f.activeTab != TabFavorites || f.searchInput.Value() == "" {
		return items
	}
	query := strings.ToLower(f.searchInput.Value())
	filtered := make([]cache.CachedSong, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Title), query) ||
			strings.Contains(strings.ToLower(item.Artist), query) ||
			strings.Contains(strings.ToLower(item.Album), query) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (f *Favorites) applyFilter() {
	f.cursor = 0
	f.scrollOffset = 0
}

func (f *Favorites) SetToastMessage(msg string) {
	f.toastMessage = msg
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
	if f.isSearching {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			switch keyMsg.String() {
			case "enter":
				f.isSearching = false
				f.searchInput.Blur()
				f.cursor = 0
				f.scrollOffset = 0
				return nil
			case "esc":
				f.isSearching = false
				f.searchInput.SetValue("")
				f.searchInput.Blur()
				f.cursor = 0
				f.scrollOffset = 0
				return nil
			case "ctrl+c":
				f.isSearching = false
				f.searchInput.SetValue("")
				f.searchInput.Blur()
				f.cursor = 0
				f.scrollOffset = 0
				return func() tea.Msg { return FavoritesMsg{Closed: true} }
			}
		}
		var cmd tea.Cmd
		f.searchInput, cmd = f.searchInput.Update(msg)
		f.applyFilter()
		return cmd
	}

	if key, ok := msg.(tea.KeyPressMsg); ok {
		f.toastMessage = ""
		visible := f.visibleRows()

		// Use offline list or regular items depending on tab
		offlineItems := f.activeOfflineItems()
		items := f.filteredItems()

		switch key.String() {
		case "esc", "q":
			if f.searchInput.Value() != "" {
				f.searchInput.SetValue("")
				f.cursor = 0
				f.scrollOffset = 0
				return nil
			}
			return func() tea.Msg { return FavoritesMsg{Closed: true} }

		case "tab":
			f.activeTab = (f.activeTab + 1) % 3
			f.cursor = 0
			f.scrollOffset = 0
			return nil

		case "/":
			if f.activeTab == TabFavorites && len(f.favorites) > 0 {
				f.isSearching = true
				f.searchInput.SetValue("")
				f.searchInput.Focus()
				f.cursor = 0
				f.scrollOffset = 0
				return nil
			}
			return nil

		case "up", "k":
			if f.activeTab == TabOffline {
				if f.cursor > 0 {
					f.cursor--
					if f.cursor < f.scrollOffset {
						f.scrollOffset = f.cursor
					}
				}
			} else if f.cursor > 0 {
				f.cursor--
				if f.cursor < f.scrollOffset {
					f.scrollOffset = f.cursor
				}
			}

		case "down", "j":
			if f.activeTab == TabOffline {
				if f.cursor < len(offlineItems)-1 {
					f.cursor++
					if f.cursor >= f.scrollOffset+visible {
						f.scrollOffset = f.cursor - visible + 1
					}
				}
			} else if f.cursor < len(items)-1 {
				f.cursor++
				if f.cursor >= f.scrollOffset+visible {
					f.scrollOffset = f.cursor - visible + 1
				}
			}

		case "d", "delete", "x":
			if f.activeTab == TabOffline {
				// Delete offline cache
				if len(f.offlineList) > 0 && f.cursor < len(f.offlineList) {
					entry := f.offlineList[f.cursor]
					_ = cache.DeleteCache(f.offlineDir, entry.Name)
					f.loadData()
					if f.cursor >= len(f.offlineList) && f.cursor > 0 {
						f.cursor--
					}
				}
			} else if len(items) > 0 && f.cursor < len(items) {
				item := items[f.cursor]
				if f.activeTab == TabFavorites {
					_ = f.cacheManager.RemoveFavoriteBySong(item.ToSong())
				} else {
					_ = f.cacheManager.RemoveBlocklist(item.SongID)
				}
				f.loadData()
				items = f.filteredItems()
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

		case "e":
			if f.activeTab == TabFavorites && len(items) > 0 && f.cursor < len(items) {
				eventID := items[f.cursor].EventID
				if f.searchInput.Value() != "" && len(items) == 1 {
					f.searchInput.SetValue("")
					f.cursor = 0
					f.scrollOffset = 0
				}
				return func() tea.Msg {
					return FavoritesMsg{EnqueueEventID: &eventID, StayOpen: true}
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
	offlineLabel := mutedStyle.Render("Offline")
	if f.activeTab == TabFavorites {
		favLabel = cursorStyle.Render("▸ Favorites")
	} else if f.activeTab == TabBlocklist {
		blockLabel = cursorStyle.Render("▸ Blocklist")
	} else {
		offlineLabel = cursorStyle.Render("▸ Offline")
	}
	tabs := favLabel + mutedStyle.Render("  │  ") + blockLabel + mutedStyle.Render("  │  ") + offlineLabel
	b.WriteString(centerStyled(tabs, contentWidth))
	b.WriteString("\n")

	// Stats line
	isFiltered := f.activeTab == TabFavorites && f.searchInput.Value() != ""
	if isFiltered {
		filtered := f.filteredItems()
		stats := mutedStyle.Render(fmt.Sprintf("%d of %d favorites", len(filtered), len(f.favorites)))
		b.WriteString(centerStyled(stats, contentWidth))
	} else if f.activeTab == TabOffline {
		var totalSize int64
		for _, entry := range f.offlineList {
			totalSize += entry.SizeBytes
		}
		stats := mutedStyle.Render(fmt.Sprintf("%d offline caches  %s total", len(f.offlineList), formatBytes(totalSize)))
		b.WriteString(centerStyled(stats, contentWidth))
	} else {
		diskSpace := f.cacheManager.GetFavoritesDiskSpace()
		stats := mutedStyle.Render(fmt.Sprintf("%d favorites  %d blocklisted  %s disk", len(f.favorites), len(f.blocklist), diskSpace))
		b.WriteString(centerStyled(stats, contentWidth))
	}
	b.WriteString("\n\n")

	// Search bar (only on Favorites tab when actively searching)
	if f.activeTab == TabFavorites && f.isSearching {
		searchWidth := contentWidth - 2
		searchBar := f.searchInput.View()
		if lipgloss.Width(searchBar) > searchWidth {
			f.searchInput.SetWidth(searchWidth)
			searchBar = f.searchInput.View()
		}
		b.WriteString("  " + searchBar)
		b.WriteString("\n\n")
	}

	// Column headers
	headerStyle := lipgloss.NewStyle().
		Foreground(f.styles.MutedStyle.GetForeground()).
		Bold(true)

	if f.activeTab == TabOffline {
		offlineCol1 := contentWidth * 25 / 100
		offlineCol2 := contentWidth * 15 / 100
		offlineCol3 := contentWidth * 15 / 100
		offlineCol4 := contentWidth * 15 / 100
		offlineCol5 := contentWidth * 15 / 100
		header := headerStyle.Render(fmt.Sprintf("  %-*s %-*s %-*s %-*s %-*s", offlineCol1, "Cache", offlineCol2, "Station", offlineCol3, "Bitrate", offlineCol4, "Duration", offlineCol5, "Size"))
		b.WriteString(header)
	} else {
		header := headerStyle.Render(fmt.Sprintf("  %-*s %-*s %-*s", songCol, "Song", artistCol, "Artist", albumCol, "Album"))
		b.WriteString(header)
	}
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(strings.Repeat("─", contentWidth)))
	b.WriteString("\n")

	// List items
	items := f.filteredItems()
	if f.activeTab == TabOffline {
		// Render offline cache list
		if len(f.offlineList) == 0 {
			b.WriteString("\n")
			b.WriteString(centerStyled(mutedStyle.Render("No offline caches. Use --cache to record."), contentWidth))
			b.WriteString("\n")
			for i := 0; i < visibleRows-2; i++ {
				b.WriteString("\n")
			}
		} else {
			offlineCol1 := contentWidth * 25 / 100
			offlineCol2 := contentWidth * 15 / 100
			offlineCol3 := contentWidth * 15 / 100
			offlineCol4 := contentWidth * 15 / 100
			offlineCol5 := contentWidth * 15 / 100

			end := f.scrollOffset + visibleRows
			if end > len(f.offlineList) {
				end = len(f.offlineList)
			}
			for i := f.scrollOffset; i < end; i++ {
				entry := f.offlineList[i]
				stationName := config.StationNames[entry.Station]
				if stationName == "" {
					stationName = fmt.Sprintf("Station %d", entry.Station)
				}
				bitrateName := config.BitrateNames[entry.Bitrate]
				if bitrateName == "" {
					bitrateName = fmt.Sprintf("%d", entry.Bitrate)
				}
				dateStr := entry.CreatedAt.Format("2006-01-02")
				row := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s",
					offlineCol1, dateStr,
					offlineCol2, stationName,
					offlineCol3, bitrateName,
					offlineCol4, cache.FormatDuration(int64(entry.ActualSeconds)),
					offlineCol5, formatBytes(entry.SizeBytes),
				)

				if i == f.cursor {
					prefix := cursorStyle.Render("▸ ")
		line := f.styles.ForegroundStyle.
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

			// Scroll indicator
			if len(f.offlineList) > visibleRows {
				scrollInfo := mutedStyle.Render(fmt.Sprintf("  %d-%d of %d", f.scrollOffset+1, min(f.scrollOffset+visibleRows, len(f.offlineList)), len(f.offlineList)))
				b.WriteString(scrollInfo)
			}
		}
	} else if len(items) == 0 {
		emptyMsg := "No favorites yet. Press f to add favorites."
		if f.activeTab == TabBlocklist {
			emptyMsg = "No blocklisted songs. Press b to blocklist."
		}
		if isFiltered {
			emptyMsg = "No matches for \"" + f.searchInput.Value() + "\""
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
		line := f.styles.ForegroundStyle.
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

	// Scroll indicator (skip for offline - already handled above)
	if f.activeTab != TabOffline && len(items) > visibleRows {
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
		helpParts = append(helpParts, accentStyle.Render("e")+mutedStyle.Render(" enqueue"))
		if f.isSearching {
			helpParts = append(helpParts, accentStyle.Render("enter")+mutedStyle.Render(" apply"))
		} else {
			helpParts = append(helpParts, accentStyle.Render("/")+mutedStyle.Render(" search"))
		}
	}
	helpParts = append(helpParts,
		accentStyle.Render("tab")+mutedStyle.Render(" switch"),
	)
	if f.searchInput.Value() != "" && !f.isSearching {
		helpParts = append(helpParts, accentStyle.Render("esc")+mutedStyle.Render(" clear filter"))
	}
	helpParts = append(helpParts,
		accentStyle.Render("esc")+mutedStyle.Render(" close"),
	)
	helpText := strings.Join(helpParts, mutedStyle.Render("  "))
	b.WriteString(centerStyled(helpText, contentWidth))

	if f.toastMessage != "" {
	toast := f.styles.CursorStyle.
		Render(f.toastMessage)
		b.WriteString("\n")
		b.WriteString(centerStyled(toast, contentWidth))
	}

	modalStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(f.styles.AccentStyle.GetForeground()).
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

// formatBytes formats bytes into human-readable string
func formatBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	} else if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	} else if b < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	}
	return fmt.Sprintf("%.2f GB", float64(b)/(1024*1024*1024))
}
