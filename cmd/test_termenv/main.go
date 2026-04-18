// +build ignore

// Test program to debug termenv terminal color detection
// Run with: go run cmd/test_termenv/main.go

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/muesli/termenv"
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
}

func isTTY(f *os.File) bool {
	// Simple check - not perfect
	fi, _ := f.Stat()
	return (fi.Mode() & os.ModeCharDevice) != 0
}