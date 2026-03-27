// Package widgets provides reusable TUI components
package widgets

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Header represents the top title bar
type Header struct {
	style lipgloss.Style
	width int
	title string
}

// NewHeader creates a new Header widget
func NewHeader(style lipgloss.Style, title string) *Header {
	return &Header{
		style: style,
		title: title,
	}
}

// SetWidth sets the width of the header
func (h *Header) SetWidth(width int) {
	h.width = width
}

// UpdateStyles updates the header style with new theme colors
func (h *Header) UpdateStyles(style lipgloss.Style) {
	h.style = style
}

// View renders the header
func (h Header) View() string {
	if h.width <= 0 {
		return h.style.Render(h.title)
	}

	// Center the title
	padding := (h.width - len(h.title)) / 2
	if padding < 0 {
		padding = 0
	}

	spaces := strings.Repeat(" ", padding)
	return h.style.Render(spaces + h.title + spaces)
}
