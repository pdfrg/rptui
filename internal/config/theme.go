// Package config handles user configuration and theme loading
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"charm.land/lipgloss/v2"
	"github.com/adrg/xdg"
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

// Built-in theme definitions
var builtinThemes = map[string]*ColorTheme{
	"catppuccin-mocha": {
		Background: "#1e1e2e",
		Foreground: "#cdd6f4",
		Accent:     "#89b4fa",
		Muted:      "#6c7086",
		Cursor:     "#f5c2e7",
	},
	"gruvbox-dark": {
		Background: "#282828",
		Foreground: "#ebdbb2",
		Accent:     "#d79921",
		Muted:      "#928374",
		Cursor:     "#fb4934",
	},
	"dark-red": {
		Background: "#220202",
		Foreground: "#dabace",
		Accent:     "#f5d943",
		Muted:      "#b4b4b5",
		Cursor:     "#ea770d",
	},
	"osaka-jade": {
		Background: "#111c18",
		Foreground: "#C1C497",
		Accent:     "#d53232",
		Muted:      "#9a9d5e",
		Cursor:     "#509475",
	},
	"synth": {
		Background: "#0d0221",
		Foreground: "#f9f9f9",
		Accent:     "#ff2a6d",
		Muted:      "#928374",
		Cursor:     "#f9fd56",
	},
	"basic": {
		Background: "#050404",
		Foreground: "#4bea45",
		Accent:     "#e02fe8",
		Muted:      "#b4b4b5",
		Cursor:     "#11c9e9",
	},
}

// ThemeNames returns the list of available built-in theme names
func ThemeNames() []string {
	return []string{
		"catppuccin-mocha", "gruvbox-dark",
		"dark-red", "osaka-jade", "synth", "basic",
	}
}

// DefaultTheme returns Catppuccin Mocha defaults
func DefaultTheme() *ColorTheme {
	return builtinThemes["catppuccin-mocha"]
}

// BuiltinTheme returns a built-in theme by name, or nil if not found
func BuiltinTheme(name string) *ColorTheme {
	if t, ok := builtinThemes[name]; ok {
		cp := *t
		return &cp
	}
	return nil
}

// LoadTheme loads theme with fallback chain
// Priority:
// 1. User-provided colors_file (if set in config)
// 2. Named built-in theme (if set and valid)
// 3. Omarchy: ~/.config/omarchy/current/theme/colors.toml
// 4. Defaults (Catppuccin Mocha)
func LoadTheme(colorsFile, themeName string) (*ColorTheme, error) {
	// Try user-provided file first
	if colorsFile != "" {
		if _, err := os.Stat(colorsFile); err == nil {
			theme, err := loadThemeFromFile(colorsFile)
			if err == nil {
				return theme, nil
			}
		}
	}

	// Try named built-in theme
	if themeName != "" {
		if t := BuiltinTheme(themeName); t != nil {
			return t, nil
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
		Background:      theme.Background,
		Foreground:      theme.Foreground,
		Accent:          theme.Accent,
		Muted:           theme.Muted,
		Cursor:          theme.Cursor,
		BackgroundStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Background)),
		ForegroundStyle: lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Foreground)),
		AccentStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Accent)),
		MutedStyle:      lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Muted)),
		CursorStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color(theme.Cursor)),
		Header: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Muted)).
			Bold(true),
		Footer: lipgloss.NewStyle().
			Foreground(lipgloss.Color(theme.Accent)).
			Bold(true),
	}
}
