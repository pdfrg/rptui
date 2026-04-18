// Package config handles user configuration and theme loading
package config

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

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

// themeColorsToml is used for parsing the full colors.toml including palette colors
type themeColorsToml struct {
	Background string `toml:"background"`
	Foreground string `toml:"foreground"`
	Accent     string `toml:"accent"`
	Muted      string `toml:"muted"`
	Cursor     string `toml:"cursor"`
	Color0     string `toml:"color0"`
	Color1     string `toml:"color1"`
	Color2     string `toml:"color2"`
	Color3     string `toml:"color3"`
	Color4     string `toml:"color4"`
	Color5     string `toml:"color5"`
	Color6     string `toml:"color6"`
	Color7     string `toml:"color7"`
	Color8     string `toml:"color8"`
	Color9     string `toml:"color9"`
	Color10    string `toml:"color10"`
	Color11    string `toml:"color11"`
	Color12    string `toml:"color12"`
	Color13    string `toml:"color13"`
	Color14    string `toml:"color14"`
	Color15    string `toml:"color15"`
}

// palette returns color0-15 as a map, normalized to #hex
func (t *themeColorsToml) palette() map[string]string {
	norm := normalizeHex
	return map[string]string{
		"0": norm(t.Color0), "1": norm(t.Color1), "2": norm(t.Color2),
		"3": norm(t.Color3), "4": norm(t.Color4), "5": norm(t.Color5),
		"6": norm(t.Color6), "7": norm(t.Color7), "8": norm(t.Color8),
		"9": norm(t.Color9), "10": norm(t.Color10), "11": norm(t.Color11),
		"12": norm(t.Color12), "13": norm(t.Color13), "14": norm(t.Color14),
		"15": norm(t.Color15),
	}
}

// ThemeStyles contains parsed lipgloss styles for the application
type ThemeStyles struct {
	// Raw color values (for creating new styles)
	Background string
	Foreground string
	Accent     string
	Muted      string
	Cursor     string

	// Special case progress bar background (only used for progress bar empty state)
	// This is the ONLY place we ever use an explicit background color in transparent modes
	ProgressBarBackground string

	// Resolved hex color values (for widgets that don't support ANSI indices)
	BackgroundHex string
	ForegroundHex string
	AccentHex     string
	MutedHex      string
	CursorHex     string

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

// loadThemeFromFile loads a theme from a TOML file, applying color fixes
func loadThemeFromFile(path string) (*ColorTheme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read theme file: %w", err)
	}

	var raw themeColorsToml
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse theme TOML: %w", err)
	}

	norm := normalizeHex
	theme := ColorTheme{
		Background: norm(raw.Background),
		Foreground: norm(raw.Foreground),
		Accent:     norm(raw.Accent),
		Cursor:     norm(raw.Cursor),
	}

	// Use color7 for muted, fallback to muted field
	if raw.Color7 != "" {
		theme.Muted = norm(raw.Color7)
	} else {
		theme.Muted = norm(raw.Muted)
	}

	// Apply defaults for missing fields
	defaults := DefaultTheme()
	if theme.Background == "" {
		theme.Background = defaults.Background
	}
	if theme.Foreground == "" {
		theme.Foreground = defaults.Foreground
	}
	if theme.Muted == "" {
		theme.Muted = defaults.Muted
	}

	// Apply color fixes: ensure accent, cursor, and foreground are all visually distinct
	palette := raw.palette()
	theme.Accent, theme.Cursor = applyColorFixes(
		theme.Foreground, theme.Background,
		theme.Accent, theme.Cursor,
		palette,
	)

	// Fallback accent/cursor to defaults if still empty
	if theme.Accent == "" {
		theme.Accent = defaults.Accent
	}
	if theme.Cursor == "" {
		theme.Cursor = defaults.Cursor
	}

	// Step 4: Muted validation — ensure muted is visually distinct from accent
	// and not too saturated (some themes set color7 to a vivid color)
	theme.Muted = applyMutedFix(theme.Accent, theme.Muted, palette)

	return &theme, nil
}

// NewThemeStyles converts ColorTheme to lipgloss styles.
// If transparentBackground is true, uses terminal's default background.
// If disableTheme is true, uses terminal's default colors for everything.
// terminalPalette provides indices for cursor/accent/muted when disable_theme is true.
func NewThemeStyles(theme *ColorTheme, transparentBackground bool, disableTheme bool, terminalPalette TerminalPaletteConfig) *ThemeStyles {
	var bg color.Color
	var fg color.Color
	var accent color.Color
	var muted color.Color
	var cursor color.Color

	// Also track raw string values for Background/Foreground/etc fields
	var bgStr string
	var fgStr string
	var accentStr string
	var mutedStr string
	var cursorStr string

	// Lipgloss v2: lipgloss.NoColor{} is the ONLY correct value that means
	// "do not draw anything on background cells - use terminal default".
	// Never pass an actual hex color if you want the terminal background to show.
	// Even if the hex exactly matches your terminal background, lipgloss will
	// unconditionally overdraw every character cell with a solid rectangle.

	if disableTheme {
		// Get terminal colors
		termFG, _, palette, err := GetTerminalColors()
		if err != nil {
			log.Printf("Warning: %v; using standard fallback", err)
			termFG = "#ffffff"
		}

		// Map palette indices
		cursorIdx := terminalPalette.Cursor
		accentIdx := terminalPalette.Accent
		mutedIdx := terminalPalette.Muted
		if cursorIdx < 0 || cursorIdx > 15 {
			cursorIdx = IdxCursor
		}
		if accentIdx < 0 || accentIdx > 15 {
			accentIdx = IdxAccent
		}
		if mutedIdx < 0 || mutedIdx > 15 {
			mutedIdx = IdxMuted
		}

		bg = lipgloss.NoColor{}
		fg = lipgloss.Color(termFG)
		cursor = lipgloss.Color(palette[cursorIdx])
		accent = lipgloss.Color(palette[accentIdx])
		muted = lipgloss.Color(palette[mutedIdx])

		bgStr = "transparent"
		fgStr = termFG
		cursorStr = palette[cursorIdx]
		accentStr = palette[accentIdx]
		mutedStr = palette[mutedIdx]
	} else if transparentBackground {
		// Use terminal's default background only (keep theme foreground colors)
		// DO NOT query terminal background color at all
		fg = lipgloss.Color(theme.Foreground)
		accent = lipgloss.Color(theme.Accent)
		muted = lipgloss.Color(theme.Muted)
		cursor = lipgloss.Color(theme.Cursor)

		bgStr = "transparent"
		fgStr = theme.Foreground
		accentStr = theme.Accent
		mutedStr = theme.Muted
		cursorStr = theme.Cursor
	} else {
		bg = lipgloss.Color(theme.Background)
		fg = lipgloss.Color(theme.Foreground)
		accent = lipgloss.Color(theme.Accent)
		muted = lipgloss.Color(theme.Muted)
		cursor = lipgloss.Color(theme.Cursor)

		bgStr = theme.Background
		fgStr = theme.Foreground
		accentStr = theme.Accent
		mutedStr = theme.Muted
		cursorStr = theme.Cursor
	}

	// Build styles
	backgroundStyle := lipgloss.NewStyle()
	foregroundStyle := lipgloss.NewStyle().Foreground(fg)
	accentStyle := lipgloss.NewStyle().Foreground(accent)
	mutedStyle := lipgloss.NewStyle().Foreground(muted)
	cursorStyle := lipgloss.NewStyle().Foreground(cursor)
	headerStyle := lipgloss.NewStyle().Foreground(muted).Bold(true)
	footerStyle := lipgloss.NewStyle().Foreground(accent).Bold(true)

	// Only apply background when we are actually using a solid theme background
	if _, isNoColor := bg.(lipgloss.NoColor); !isNoColor {
		backgroundStyle = backgroundStyle.Background(bg)
		foregroundStyle = foregroundStyle.Background(bg)
		accentStyle = accentStyle.Background(bg)
		mutedStyle = mutedStyle.Background(bg)
		cursorStyle = cursorStyle.Background(bg)
		headerStyle = headerStyle.Background(bg)
		footerStyle = footerStyle.Background(bg)
	}

	// Special case: query terminal background ONLY for progress bar usage
	// We never use this anywhere else in styles - only for progress bar empty state
	progressBarBg := ""
	if transparentBackground || disableTheme {
		_, termBG, _, err := GetTerminalColors()
		if err == nil && termBG != "" && len(termBG) == 7 && termBG[0] == '#' {
			progressBarBg = termBG
		} else {
			// Neutral fallback that looks acceptable on all background shades
			progressBarBg = "#222222"
		}
	} else {
		progressBarBg = bgStr
	}

	// Build base styles WITHOUT background first
	backgroundStyle = lipgloss.NewStyle()
	foregroundStyle = lipgloss.NewStyle().Foreground(fg)
	accentStyle = lipgloss.NewStyle().Foreground(accent)
	mutedStyle = lipgloss.NewStyle().Foreground(muted)
	cursorStyle = lipgloss.NewStyle().Foreground(cursor)
	headerStyle = lipgloss.NewStyle().Foreground(muted).Bold(true)
	footerStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)

	// ONLY add background when we are NOT in terminal default modes
	// In lipgloss v2 there is NO other way to get proper terminal background
	// NoColor{} is broken and always renders black. Omitting .Background() is the only working method.
	if !transparentBackground && !disableTheme {
		backgroundStyle = backgroundStyle.Background(bg)
		foregroundStyle = foregroundStyle.Background(bg)
		accentStyle = accentStyle.Background(bg)
		mutedStyle = mutedStyle.Background(bg)
		cursorStyle = cursorStyle.Background(bg)
		headerStyle = headerStyle.Background(bg)
		footerStyle = footerStyle.Background(bg)
	}

	return &ThemeStyles{
		Background:            bgStr,
		Foreground:            fgStr,
		Accent:                accentStr,
		Muted:                 mutedStr,
		Cursor:                cursorStr,
		BackgroundHex:         normalizeHex(bgStr),
		ForegroundHex:         normalizeHex(fgStr),
		AccentHex:             normalizeHex(accentStr),
		MutedHex:              normalizeHex(mutedStr),
		CursorHex:             normalizeHex(cursorStr),
		ProgressBarBackground: progressBarBg,
		BackgroundStyle:       backgroundStyle,
		ForegroundStyle:       foregroundStyle,
		AccentStyle:           accentStyle,
		MutedStyle:            mutedStyle,
		CursorStyle:           cursorStyle,
		Header:                headerStyle,
		Footer:                footerStyle,
	}
}

// --- Color math helpers for theme color fixing ---

func normalizeHex(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "transparent" {
		return "transparent"
	}
	if strings.HasPrefix(s, "0x") {
		return "#" + s[2:]
	}
	return s
}

func parseHexColor(s string) (r, g, b int, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "transparent" {
		return 0, 0, 0, false
	}
	s = strings.TrimPrefix(s, "0x")
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	_, err := fmt.Sscanf(s, "%02x%02x%02x", &r, &g, &b)
	return r, g, b, err == nil
}

func weightedDistance(h1, h2 string) float64 {
	r1, g1, b1, ok1 := parseHexColor(h1)
	r2, g2, b2, ok2 := parseHexColor(h2)
	if !ok1 || !ok2 {
		return -1
	}
	dr, dg, db := float64(r1-r2), float64(g1-g2), float64(b1-b2)
	return math.Sqrt(0.299*dr*dr + 0.587*dg*dg + 0.114*db*db)
}

func strikingScore(hex string) float64 {
	r, g, b, ok := parseHexColor(hex)
	if !ok {
		return 0
	}
	maxC := float64(max(r, g, b)) / 255.0
	minC := float64(min(r, g, b)) / 255.0
	var sat float64
	if maxC > 0 {
		sat = (maxC - minC) / maxC
	}
	return maxC * sat
}

func minDistBetween(hex string, list []string) float64 {
	minD := -1.0
	for _, other := range list {
		if other == "" {
			continue
		}
		d := weightedDistance(hex, other)
		if minD < 0 || (d >= 0 && d < minD) {
			minD = d
		}
	}
	return minD
}

func isExcluded(hex string, exclude []string) bool {
	for _, e := range exclude {
		if strings.EqualFold(hex, e) {
			return true
		}
	}
	return false
}

// findReplacement picks from theme's color1-15 palette.
// Candidate must not be in exclude and must be wD>30 from every mustDifferFrom color.
// If preferStriking, picks most vivid; otherwise picks first available.
func findReplacement(colors map[string]string, exclude, mustDifferFrom []string, preferStriking bool) string {
	indices := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13", "14", "15"}
	var candidates []string
	for _, idx := range indices {
		c := colors[idx]
		if c == "" || isExcluded(c, exclude) {
			continue
		}
		if minDistBetween(c, mustDifferFrom) < 30 {
			continue
		}
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return ""
	}
	if !preferStriking {
		return candidates[0]
	}
	best, bestScore := "", -1.0
	for _, c := range candidates {
		if s := strikingScore(c); s > bestScore {
			best, bestScore = c, s
		}
	}
	return best
}

// applyColorFixes ensures accent, cursor, and foreground are all visually distinct.
// Returns (fixedAccent, fixedCursor). Original values are kept if no fix is needed.
//
// Step 1: Cursor vs foreground — if too similar (wD<20), find replacement.
// Step 2: Accent vs cursor — must always differ for rptui. If identical (wD<0.1), find replacement.
// Step 3: Accent vs foreground — if too similar (wD<20), find striking replacement.
func applyColorFixes(fg, bg, accent, cursor string, palette map[string]string) (string, string) {
	curCursor := cursor
	curAccent := accent
	exclude := []string{fg, bg}

	// Step 1: Cursor vs foreground (must be visibly different)
	if cursor != "" && fg != "" && weightedDistance(cursor, fg) >= 0 && weightedDistance(cursor, fg) < 20 {
		if fix := findReplacement(palette, exclude, []string{fg, cursor, accent}, false); fix != "" {
			curCursor = fix
			exclude = append(exclude, cursor, fix)
		}
	}

	// Step 2: Accent vs cursor (must always differ in rptui)
	if accent != "" && curCursor != "" && weightedDistance(accent, curCursor) >= 0 && weightedDistance(accent, curCursor) < 0.1 {
		if fix := findReplacement(palette, exclude, []string{fg, curCursor}, true); fix != "" {
			curAccent = fix
			exclude = append(exclude, accent, fix)
		}
	}

	// Step 3: Accent vs foreground (must be visibly different)
	if accent != "" && fg != "" && curAccent == accent &&
		weightedDistance(accent, fg) >= 0 && weightedDistance(accent, fg) < 20 {
		if fix := findReplacement(palette, exclude, []string{fg, curCursor}, true); fix != "" {
			curAccent = fix
		}
	}

	return curAccent, curCursor
}

// saturation returns the saturation of an RGB color (0=gray, 1=fully saturated).
func saturation(r, g, b int) float64 {
	maxC := float64(max(r, g, b))
	minC := float64(min(r, g, b))
	if maxC == 0 {
		return 0
	}
	return (maxC - minC) / maxC
}

// clampByte clamps an int to valid byte range [0, 255].
func clampByte(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// desaturateToTarget desaturates a hex color to approximately targetSat
// by blending each channel toward neutral gray (#808080).
// Preserves hue direction and lightness character while reducing saturation.
func desaturateToTarget(hex string, targetSat float64) string {
	r, g, b, ok := parseHexColor(hex)
	if !ok {
		return hex
	}
	sat := saturation(r, g, b)
	if sat <= targetSat {
		return hex
	}

	maxC := float64(max(r, g, b))
	minC := float64(min(r, g, b))

	// Solve for blend factor where new_sat ≈ targetSat
	// After blending toward #80 (128) by factor f:
	//   sat' = (max-min)*(1-f) / (max*(1-f) + 128*f)
	denom := (maxC - minC) - targetSat*maxC + 128*targetSat
	if denom <= 0 {
		return desaturateGray(hex, 0.8)
	}
	blend := ((maxC - minC) - targetSat*maxC) / denom
	if blend < 0 {
		blend = 0
	}
	if blend > 0.9 {
		blend = 0.9
	}
	return desaturateGray(hex, blend)
}

func desaturateGray(hex string, blendFactor float64) string {
	r, g, b, ok := parseHexColor(hex)
	if !ok {
		return hex
	}
	nr := int(math.Round(float64(r)*(1-blendFactor) + 128*blendFactor))
	ng := int(math.Round(float64(g)*(1-blendFactor) + 128*blendFactor))
	nb := int(math.Round(float64(b)*(1-blendFactor) + 128*blendFactor))
	return fmt.Sprintf("#%02x%02x%02x", clampByte(nr), clampByte(ng), clampByte(nb))
}

// applyMutedFix validates the muted color and fixes it if:
//  1. It's too similar to accent (weightedDistance < 30)
//  2. It's too saturated for a muted color (saturation > 0.35)
//
// Fix strategy (in order):
//  1. Try color8 if it's desaturated (sat<0.35) and distinct from accent
//  2. Desaturate color7 to target saturation 0.25 (preserves hue/character)
//  3. Fall back to built-in default muted
func applyMutedFix(accent, muted string, palette map[string]string) string {
	if muted == "" {
		return muted
	}

	r, g, b, ok := parseHexColor(muted)
	if !ok {
		return muted
	}

	muSat := saturation(r, g, b)
	muDist := -1.0
	if accent != "" {
		muDist = weightedDistance(accent, muted)
	}

	needsFix := false
	if accent != "" && muDist >= 0 && muDist < 30 {
		needsFix = true
	} else if muSat > 0.35 {
		needsFix = true
	}

	if !needsFix {
		return muted
	}

	// Try color8 first
	c8 := palette["8"]
	if c8 != "" {
		r8, g8, b8, ok8 := parseHexColor(c8)
		if ok8 {
			c8sat := saturation(r8, g8, b8)
			if c8sat < 0.35 && (accent == "" || weightedDistance(accent, c8) >= 30) {
				return c8
			}
		}
	}

	// Desaturate color7 to target saturation 0.25
	desat := desaturateToTarget(muted, 0.25)
	if desat != muted {
		return desat
	}

	// Last resort: built-in default
	return DefaultTheme().Muted
}
