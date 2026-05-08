package tui

import (
	"fmt"
	"strings"

	"github.com/blacktop/go-termimg"
)

// ClearImageAtCurrentPosition returns the appropriate clear sequence for the given protocol
// to clear an image at the specified dimensions (in character cells).
// IMPORTANT: This function assumes the cursor is already positioned at the top-left
// of the image area. It clears only the rectangular image area, not entire rows.
// For Halfblocks, dimensions are stored as cells already (width/2, height/2 from app.go).
func ClearImageAtCurrentPosition(protocol termimg.Protocol, width, height int) string {
	switch protocol {
	case termimg.Sixel:
		return buildSixelClearAtCurrentPosition(width, height)
	case termimg.ITerm2:
		return buildITerm2ClearAtCurrentPosition(width, height)
	case termimg.Halfblocks:
		return buildHalfblocksClearAtCurrentPosition(width, height)
	case termimg.Kitty:
		// Kitty uses ClearAllString() which is handled separately
		return ""
	default:
		return ""
	}
}

// buildSixelClearAtCurrentPosition clears the image area by printing spaces
// Assumes cursor is at image top-left, only overwrites the image area (width x height)
func buildSixelClearAtCurrentPosition(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	var b strings.Builder
	clearLine := strings.Repeat(" ", width)
	// Print spaces for each row of the image area
	for i := 0; i < height; i++ {
		b.WriteString(clearLine)
		if i < height-1 {
			b.WriteString("\x1b[B") // Move down one line
		}
	}
	// Move back up to original position (top-left of image area)
	fmt.Fprintf(&b, "\x1b[%dA", height)
	return b.String()
}

// buildITerm2ClearAtCurrentPosition clears the image area using iTerm2-compatible sequences
// Assumes cursor is at image top-left, only overwrites the image area (width x height)
func buildITerm2ClearAtCurrentPosition(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	var b strings.Builder
	clearLine := strings.Repeat(" ", width)
	for i := 0; i < height; i++ {
		b.WriteString(clearLine)
		if i < height-1 {
			b.WriteString("\x1b[B") // Move down one line
		}
	}
	// Move back up to original position
	fmt.Fprintf(&b, "\x1b[%dA", height)
	return b.String()
}

// buildHalfblocksClearAtCurrentPosition clears the image area by overwriting with spaces
func buildHalfblocksClearAtCurrentPosition(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	// For halfblocks, dimensions are already in character cells
	// (stored as width/2, height/2 in handleImageLoaded)
	clearLine := strings.Repeat(" ", width)
	var b strings.Builder
	for i := 0; i < height; i++ {
		b.WriteString(clearLine)
		if i < height-1 {
			b.WriteString("\n")
		}
	}
	// Move cursor back up to original position (after the last newline)
	fmt.Fprintf(&b, "\x1b[%dA", height)
	return b.String()
}
