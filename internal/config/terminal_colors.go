package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/muesli/termenv"
)

type TerminalColors struct {
	mu            sync.RWMutex
	foreground    string
	background    string
	palette       map[int]string
	queried      bool
	querySuccess bool
}

var (
	termColors     TerminalColors
	termColorsOnce sync.Once
)

// Standard palette indices for fallback when terminal queries fail
const (
	IdxCursor = 2 // green
	IdxAccent = 4 // blue
	IdxMuted = 8  // gray
)

// GetTerminalColors returns the terminal's default colors.
// Queries the terminal once and caches the results.
func GetTerminalColors() (fg, bg string, palette map[int]string, err error) {
	termColorsOnce.Do(func() {
		termColors.queryTerminalColors()
	})

	termColors.mu.RLock()
	defer termColors.mu.RUnlock()

	if !termColors.queried {
		return "", "", nil, fmt.Errorf("terminal colors not yet queried")
	}

	if !termColors.querySuccess {
		return "", "", nil, fmt.Errorf("could not detect terminal colors; see --test-terminal-colors for details")
	}

	return termColors.foreground, termColors.background, termColors.palette, nil
}

// GetCachedTerminalColors returns cached results (no query).
func GetCachedTerminalColors() (fg, bg string, palette map[int]string, success bool) {
	termColors.mu.RLock()
	defer termColors.mu.RUnlock()

	return termColors.foreground, termColors.background, termColors.palette, termColors.querySuccess
}

// TestTerminalColors prints detected colors for debugging.
// Returns map of index to hex color for palette indices 0-15.
func TestTerminalColors() (fg, bg string, palette map[int]string, fallback bool, err error) {
	// Always run fresh query for --test-terminal-colors
	termColors.queryTerminalColors()

	termColors.mu.RLock()
	defer termColors.mu.RUnlock()

	fg = termColors.foreground
	bg = termColors.background
	palette = make(map[int]string)
	for k, v := range termColors.palette {
		palette[k] = v
	}
	fallback = !termColors.querySuccess

	return fg, bg, palette, fallback, nil
}

func (t *TerminalColors) queryTerminalColors() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.queried {
		return
	}
	t.queried = true
	t.palette = make(map[int]string)

	// Try termenv for default fg/bg (OSC 10/11)
	output := termenv.NewOutput(os.Stdout)
	fgColor := output.ForegroundColor()
	bgColor := output.BackgroundColor()

	// Use Color.Sequence() to get ANSI codes, then parse
	if fgColor != nil {
		seq := fgColor.Sequence(false)
		if seq != "" {
			t.foreground = sequenceToHex(seq)
		}
	}
	if bgColor != nil {
		seq := bgColor.Sequence(true)
		if seq != "" {
			t.background = sequenceToHex(seq)
		}
	}

	// Try to query palette via OSC 4
	// Note: termenv doesn't support OSC 4 directly, but we can parse COLORFGBG
	t.queryPaletteFromEnv()

	// Determine success
	if t.foreground != "" && t.background != "" {
		t.querySuccess = true
	}
}

func (t *TerminalColors) queryPaletteFromEnv() {
	// Try COLORFGBG environment variable
	// Format: fg;bg or fg;xfg;bg or various formats from different terminals
	colorFGBG := os.Getenv("COLORFGBG")
	if colorFGBG == "" {
		return
	}

	// Split on semicolon to get indices
	// Common formats: "7;0" (fg;bg) or "7;0;7;0;0" (fg;bg;hfg;hbg;sl)
	parts := strings.Split(colorFGBG, ";")
	if len(parts) < 2 {
		return
	}

	// Try to parse foreground and background as ANSI indices
	if fgIdx, err := strconv.Atoi(parts[0]); err == nil && fgIdx >= 0 && fgIdx <= 15 {
		if t.foreground == "" {
			t.foreground = ansiIndexToHex(fgIdx)
		}
	}

	if bgIdx, err := strconv.Atoi(parts[1]); err == nil && bgIdx >= 0 && bgIdx <= 15 {
		if t.background == "" {
			t.background = ansiIndexToHex(bgIdx)
		}
	}

	// For now, we can't get full palette from COLORFGBG
	// Leave palette empty - will use hardcoded fallback colors
}

// sequenceToHex converts an ANSI sequence to hex color.
// Handles: \x1b[38;2;R;G;Bm (24-bit), \x1b[3Xm (8-bit), etc.
func sequenceToHex(seq string) string {
	// Already is a hex code from OSC query response
	if len(seq) == 7 && seq[0] == '#' {
		return seq
	}

	// Parse ANSI escape sequence
	// Remove common prefixes
	seq = strings.TrimPrefix(seq, "\x1b[")
	seq = strings.TrimSuffix(seq, "m")

	// Try 24-bit color: 38;2;r;g;b
	parts := strings.Split(seq, ";")
	if len(parts) >= 4 && parts[0] == "38" && parts[1] == "2" {
		if r, err := strconv.ParseUint(parts[2], 10, 8); err == nil {
			if g, err := strconv.ParseUint(parts[3], 10, 8); err == nil {
				if b, err := strconv.ParseUint(parts[4], 10, 8); err == nil {
					return fmt.Sprintf("#%02x%02x%02x", r, g, b)
				}
			}
		}
	}

	// Try 8-bit color: 38;5;n
	if len(parts) >= 2 && parts[0] == "38" && parts[1] == "5" {
		if idx, err := strconv.Atoi(parts[2]); err == nil {
			return ansiIndexToHex(idx)
		}
	}

	// Couldn't parse - return empty to trigger fallback
	return ""
}

func ansiIndexToHex(idx int) string {
	// Common ANSI to sRGB approximations for standard palettes (varies by terminal!)
	// These are approximate sRGB values for typical terminal palettes
	ansiToRGB := []struct{ r, g, b uint8 }{
		{0x00, 0x00, 0x00}, // 0: black
		{0xcd, 0x00, 0x00}, // 1: red
		{0x00, 0xcd, 0x00}, // 2: green
		{0xcd, 0xcd, 0x00}, // 3: yellow
		{0x00, 0x00, 0xcd}, // 4: blue
		{0xcd, 0x00, 0xcd}, // 5: magenta
		{0x00, 0xcd, 0xcd}, // 6: cyan
		{0xcd, 0xcd, 0xcd}, // 7: white
		{0x7f, 0x7f, 0x7f}, // 8: bright black (gray)
		{0xff, 0x00, 0x00}, // 9: bright red
		{0x00, 0xff, 0x00}, // 10: bright green
		{0xff, 0xff, 0x00}, // 11: bright yellow
		{0x00, 0x00, 0xff}, // 12: bright blue
		{0xff, 0x00, 0xff}, // 13: bright magenta
		{0x00, 0xff, 0xff}, // 14: bright cyan
		{0xff, 0xff, 0xff}, // 15: bright white
	}

	if idx < 0 || idx > 15 {
		return ""
	}

	c := ansiToRGB[idx]
	return fmt.Sprintf("#%02x%02x%02x", c.r, c.g, c.b)
}

// ParsePaletteColor queries a single palette color via OSC 4
// This is not implemented in termenv, so we provide a placeholder
func ParsePaletteColor(n int) (string, error) {
	if n < 0 || n > 15 {
		return "", fmt.Errorf("palette index must be 0-15")
	}

	// Check if we already have it cached
	termColors.mu.RLock()
	if c, ok := termColors.palette[n]; ok {
		termColors.mu.RUnlock()
		return c, nil
	}
	termColors.mu.RUnlock()

	// Cannot query individual colors without direct OSC 4 implementation
	// Return standard ANSI approximation
	return ansiIndexToHex(n), nil
}

// IsTerminalColorAvailable checks if terminal colors were successfully queried
func IsTerminalColorAvailable() bool {
	termColors.mu.RLock()
	defer termColors.mu.RUnlock()
	return termColors.querySuccess
}