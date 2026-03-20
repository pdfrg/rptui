// Package config handles user configuration and theme loading
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"charm.land/lipgloss/v2"
	"github.com/pelletier/go-toml/v2"
)

// ColorTheme represents a color scheme loaded from TOML
type ColorTheme struct {
	Background string `toml:"background"`
	Foreground string `toml:"foreground"`
	Accent     string `toml:"accent"`
	Muted      string `toml:"muted"`
	Cursor     string `toml:"cursor"`
}

// ThemeStyles contains parsed lipgloss styles for the application
type ThemeStyles struct {
	// Raw color values (for creating new styles)
	Background string
	Foreground string
	Accent     string
	Muted      string
	Cursor     string

	// Pre-built styles
	BackgroundStyle lipgloss.Style
	ForegroundStyle lipgloss.Style
	AccentStyle     lipgloss.Style
	MutedStyle      lipgloss.Style
	CursorStyle     lipgloss.Style
	Header          lipgloss.Style
	Footer          lipgloss.Style
}

// DefaultTheme returns Catppuccin Mocha defaults
func DefaultTheme() *ColorTheme {
	return &ColorTheme{
		Background: "#1e1e2e",
		Foreground: "#cdd6f4",
		Accent:     "#89b4fa",
		Muted:      "#6c7086",
		Cursor:     "#f5c2e7",
	}
}

// LoadTheme loads theme from config file with fallback chain
// Priority:
// 1. User-provided colors_file (if set in config)
// 2. Omarchy: ~/.config/omarchy/current/theme/colors.toml
// 3. Defaults (Catppuccin Mocha)
func LoadTheme(colorsFile string) (*ColorTheme, error) {
	// Try user-provided file first
	if colorsFile != "" {
		if _, err := os.Stat(colorsFile); err == nil {
			theme, err := loadThemeFromFile(colorsFile)
			if err == nil {
				return theme, nil
			}
		}
	}

	// Try Omarchy theme
	omarchyPath := filepath.Join(xdg.ConfigHome, "omarchy", "current", "theme", "colors.toml")
	if _, err := os.Stat(omarchyPath); err == nil {
		theme, err := loadThemeFromFile(omarchyPath)
		if err == nil {
			return theme, nil
		}
	}

	// Return defaults
	return DefaultTheme(), nil
}

// loadThemeFromFile loads a theme from a TOML file
func loadThemeFromFile(path string) (*ColorTheme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read theme file: %w", err)
	}

	var theme ColorTheme
	if err := toml.Unmarshal(data, &theme); err != nil {
		return nil, fmt.Errorf("failed to parse theme TOML: %w", err)
	}

	// Validate and apply defaults for missing fields
	defaults := DefaultTheme()
	if theme.Background == "" {
		theme.Background = defaults.Background
	}
	if theme.Foreground == "" {
		theme.Foreground = defaults.Foreground
	}
	if theme.Accent == "" {
		theme.Accent = defaults.Accent
	}
	if theme.Muted == "" {
		theme.Muted = defaults.Muted
	}
	if theme.Cursor == "" {
		theme.Cursor = defaults.Cursor
	}

	return &theme, nil
}

// NewThemeStyles converts ColorTheme to lipgloss styles
func NewThemeStyles(theme *ColorTheme) *ThemeStyles {
	return &ThemeStyles{
		Background: theme.Background,
		Foreground: theme.Foreground,
		Accent:     theme.Accent,
		Muted:      theme.Muted,
		Cursor:     theme.Cursor,
		BackgroundStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Background)),
		ForegroundStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Foreground)),
		AccentStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent)),
		MutedStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted)),
		CursorStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Cursor)),
		Header: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Muted)).
			Bold(true),
		Footer: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Accent)).
			Bold(true),
	}
}
