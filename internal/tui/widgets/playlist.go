// Package widgets provides reusable TUI components
package widgets

import (
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"rptui-bubbletea/internal/config"
)

// Playlist represents the playlist table widget
type Playlist struct {
	table  table.Model
	styles *config.ThemeStyles
	width  int
	height int
}

// NewPlaylist creates a new Playlist widget
func NewPlaylist(styles *config.ThemeStyles) *Playlist {
	columns := []table.Column{
		{Title: "#", Width: 3},
		{Title: "Song", Width: 30},
		{Title: "Artist", Width: 20},
		{Title: "Duration", Width: 8},
		{Title: "Album", Width: 25},
		{Title: "Year", Width: 5},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithHeight(10),
		table.WithWidth(100),
	)

	// Apply styles - header background = lightened background color, no border
	headerBg := lightenColor(styles.Background, 0.30)
	s := table.DefaultStyles()
	s.Header = s.Header.
		Bold(false).
		Foreground(lipgloss.Color(styles.Muted)).
		Background(lipgloss.Color(headerBg))
	s.Selected = s.Selected.
		Foreground(lipgloss.Color(styles.Cursor)).
		Bold(true)
	t.SetStyles(s)

	return &Playlist{
		table:  t,
		styles: styles,
	}
}

// SetSize sets the dimensions of the playlist table
func (p *Playlist) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.table.SetWidth(width)
	p.table.SetHeight(height)
}

// SetRows sets the rows of the playlist table
func (p *Playlist) SetRows(rows []table.Row) {
	p.table.SetRows(rows)
}

// SetCursor sets the selected row in the playlist table
func (p *Playlist) SetCursor(cursor int) {
	p.table.SetCursor(cursor)
}

// Update handles messages
func (p *Playlist) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	p.table, cmd = p.table.Update(msg)
	return cmd
}

// View renders the playlist table
func (p Playlist) View() string {
	return p.table.View()
}
