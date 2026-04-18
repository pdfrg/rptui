// +build ignore

// Test program to debug termenv terminal color detection
// Run with: go run cmd/test_termenv/main.go

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/muesli/termenv"
	"golang.org/x/term"
)

func main() {
	fmt.Println("=== Termenv Terminal Color Test ===")
	fmt.Println()

	// Create output with stdout
	output := termenv.NewOutput(os.Stdout)

	// Get colors
	fg := output.ForegroundColor()
	bg := output.BackgroundColor()

	fmt.Printf("Foreground type: %T\n", fg)
	fmt.Printf("Background type: %T\n", bg)
	fmt.Println()

	// Check if colors are nil
	fmt.Printf("Foreground is nil: %v\n", fg == nil)
	fmt.Printf("Background is nil: %v\n", bg == nil)
	fmt.Println()

	// Try to get ANSI sequences
	if fg != nil {
		seq := fg.Sequence(false)
		fmt.Printf("Foreground Sequence: %q\n", seq)
	}
	if bg != nil {
		seq := bg.Sequence(true)
		fmt.Printf("Background Sequence: %q\n", seq)
	}
	fmt.Println()

	// Check TTY
	fmt.Printf("Is TTY: %v\n", isTTY(os.Stdout))
	fmt.Println()

	// Check profile
	fmt.Printf("Color Profile: %v\n", output.ColorProfile())
	fmt.Printf("Env NoColor: %v\n", output.EnvNoColor())
	fmt.Println()

	// Try TTY version with termenv
	fmt.Println("=== Trying TTY (termenv) ===")
	tty := output.TTY()
	if tty != nil {
		ttyOutput := termenv.NewOutput(tty)
		ttyFg := ttyOutput.ForegroundColor()
		ttyBg := ttyOutput.BackgroundColor()
		fmt.Printf("TTY FG: %T, seq: %q\n", ttyFg, ttyFg.Sequence(false))
		fmt.Printf("TTY BG: %T, seq: %q\n", ttyBg, ttyBg.Sequence(true))
	}

	// Try direct OSC query via TTY
	fmt.Println()
	fmt.Println("=== Direct OSC Query via TTY ===")
	tty = output.TTY()
	if tty != nil {
		// Send OSC 10 and 11 queries
		tty.Write([]byte("\x1b]10;?\x1b\\\x1b]11;?\x1b\\"))
		// Give time to respond
		time.Sleep(500 * time.Millisecond)
		// Read response
		buf := make([]byte, 1024)
		n, _ := tty.Read(buf)
		fmt.Printf("Response: %q\n", string(buf[:n]))
	}

	// Query palette colors 0-15
	fmt.Println()
	fmt.Println("=== Palette Colors (OSC 4) ===")
	tty = output.TTY()
	if tty != nil {
		for i := 0; i < 16; i++ {
			color := queryPaletteColor(tty, i)
			fmt.Printf("Color %d: %s\n", i, color)
		}
	}
}

func isTTY(f *os.File) bool {
	// Simple check - not perfect
	fi, _ := f.Stat()
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// queryPaletteColor sends OSC 4 query for a specific color index and reads the response
func queryPaletteColor(tty termenv.File, index int) string {
	// Get the underlying file descriptor for termios operations
	fd := int(tty.Fd())

	// Save terminal state and put in raw mode to disable echo
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return ""
	}
	defer term.Restore(fd, oldState)

	// Send OSC 4;n;? query - ask terminal for color at index n
	query := fmt.Sprintf("\x1b]4;%d;?\x1b\\", index)
	_, err = tty.Write([]byte(query))
	if err != nil {
		return ""
	}

	// Give terminal time to respond
	time.Sleep(50 * time.Millisecond)

	// Read response
	buf := make([]byte, 1024)
	n, err := tty.Read(buf)
	if err != nil || n == 0 {
		return ""
	}

	response := string(buf[:n])
	return parsePaletteResponse(response)
}

// parsePaletteResponse parses OSC 4 response into hex color
func parsePaletteResponse(response string) string {
	// Look for rgb: or # format
	idx := strings.Index(response, "rgb:")
	if idx == -1 {
		idx = strings.Index(response, "#")
		if idx == -1 {
			return ""
		}
		// Parse #RRGGBB format
		hex := response[idx+1 : idx+7]
		if len(hex) == 6 {
			return "#" + hex
		}
		return ""
	}

	// Parse rgb:R/G/B format
	rest := response[idx+4:]
	if endIdx := strings.Index(rest, "\x1b"); endIdx != -1 {
		rest = rest[:endIdx]
	}
	if endIdx := strings.Index(rest, "\\"); endIdx != -1 {
		rest = rest[:endIdx]
	}

	parts := strings.Split(rest, "/")
	if len(parts) >= 3 {
		// Convert from 4-digit hex (0000-ffff) to 2-digit
		r := hexToHex2(parts[0])
		g := hexToHex2(parts[1])
		b := hexToHex2(parts[2])
		if r != "" && g != "" && b != "" {
			return "#" + r + g + b
		}
	}

	return ""
}

// hexToHex2 converts 4-digit hex to 2-digit hex
func hexToHex2(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return ""
	}
	if len(s) >= 2 {
		return s[:2]
	}
	return ""
}