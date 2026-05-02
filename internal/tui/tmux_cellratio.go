package tui

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

func inTmux() bool {
	return os.Getenv("TMUX") != "" || os.Getenv("TERM_PROGRAM") == "tmux"
}

func wrapTmuxPassthrough(output string) string {
	return "\x1bPtmux;\x1b" + strings.ReplaceAll(output, "\x1b", "\x1b\x1b") + "\x1b\\"
}

// correctCellRatioForTmux fixes the cell ratio when running inside tmux.
// go-termimg's QueryFontSize() divides pixel dimensions from CSI 14t by the
// tmux pane's character dimensions (from TIOCGWINSZ), producing wrong results
// because the pixel dimensions reflect the full outer terminal window while
// the character dimensions reflect the inner tmux pane.
//
// This function uses two strategies:
// 1. CSI 16t (character cell size in pixels) — this passes through tmux
// correctly and returns the outer terminal's actual per-cell pixel size,
// avoiding the division mismatch entirely.
// 2. tmux client dimensions — runs `tmux display -p` to get the outer
// terminal's character count, then pairs it with CSI 14t pixel dimensions
// for the division.
//
// Returns (fontWidth, fontHeight, true) if a correction was applied,
// or (0, 0, false) if no correction is needed or available.
func correctCellRatioForTmux(logger *log.Logger) (int, int, bool) {
	if !inTmux() {
		return 0, 0, false
	}

	if w, h, ok := queryCSI16tViaTmux(); ok && w > 0 && h > 0 {
		logger.Printf("tmux cellRatio fix: CSI 16t returned %dx%d", w, h)
		return w, h, true
	}
	logger.Printf("tmux cellRatio fix: CSI 16t failed, trying tmux client dimensions")

	clientW, clientH, ok := getTmuxClientDimensions()
	if !ok {
		logger.Printf("tmux cellRatio fix: tmux client dimensions failed")
		return 0, 0, false
	}

	pixelW, pixelH, ok := queryCSI14tViaTmux()
	if !ok {
		logger.Printf("tmux cellRatio fix: CSI 14t via tmux failed")
		return 0, 0, false
	}

	fontW := pixelW / clientW
	fontH := pixelH / clientH

	if fontW < 4 || fontW > 50 || fontH < 4 || fontH > 50 {
		logger.Printf("tmux cellRatio fix: computed font size %dx%d out of range", fontW, fontH)
		return 0, 0, false
	}

	logger.Printf("tmux cellRatio fix: tmux client dims %dx%d + CSI 14t %dx%d -> font %dx%d",
		clientW, clientH, pixelW, pixelH, fontW, fontH)
	return fontW, fontH, true
}

func queryCSI16tViaTmux() (width, height int, ok bool) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return 0, 0, false
	}
	defer tty.Close()

	oldState, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return 0, 0, false
	}
	defer term.Restore(int(tty.Fd()), oldState)

	query := wrapTmuxPassthrough("\x1b[16t")

	if _, err := tty.WriteString(query); err != nil {
		return 0, 0, false
	}

	responseChan := make(chan [2]int, 1)
	go func() {
		buf := make([]byte, 64)
		n, err := tty.Read(buf)
		if err == nil && n > 0 {
			response := string(buf[:n])
			if strings.Contains(response, "[6;") && strings.Contains(response, "t") {
				parts := strings.Split(response, ";")
				if len(parts) >= 3 {
					var h, w int
					fmt.Sscanf(parts[1], "%d", &h)
					fmt.Sscanf(parts[2], "%dt", &w)
					if w > 0 && h > 0 {
						responseChan <- [2]int{w, h}
						return
					}
				}
			}
		}
		responseChan <- [2]int{0, 0}
	}()

	select {
	case result := <-responseChan:
		return result[0], result[1], result[0] > 0 && result[1] > 0
	case <-time.After(200 * time.Millisecond):
		return 0, 0, false
	}
}

func queryCSI14tViaTmux() (width, height int, ok bool) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return 0, 0, false
	}
	defer tty.Close()

	oldState, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return 0, 0, false
	}
	defer term.Restore(int(tty.Fd()), oldState)

	query := wrapTmuxPassthrough("\x1b[14t")

	if _, err := tty.WriteString(query); err != nil {
		return 0, 0, false
	}

	responseChan := make(chan [2]int, 1)
	go func() {
		buf := make([]byte, 64)
		n, err := tty.Read(buf)
		if err == nil && n > 0 {
			response := string(buf[:n])
			if strings.Contains(response, "[4;") && strings.Contains(response, "t") {
				parts := strings.Split(response, ";")
				if len(parts) >= 3 {
					var h, w int
					fmt.Sscanf(parts[1], "%d", &h)
					fmt.Sscanf(parts[2], "%dt", &w)
					if w > 0 && h > 0 {
						responseChan <- [2]int{w, h}
						return
					}
				}
			}
		}
		responseChan <- [2]int{0, 0}
	}()

	select {
	case result := <-responseChan:
		return result[0], result[1], result[0] > 0 && result[1] > 0
	case <-time.After(200 * time.Millisecond):
		return 0, 0, false
	}
}

func getTmuxClientDimensions() (width, height int, ok bool) {
	out, err := exec.Command("tmux", "display", "-p", "#{client_width} #{client_height}").Output()
	if err != nil {
		return 0, 0, false
	}

	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 2 {
		return 0, 0, false
	}

	w, err := strconv.Atoi(fields[0])
	if err != nil || w <= 0 {
		return 0, 0, false
	}

	h, err := strconv.Atoi(fields[1])
	if err != nil || h <= 0 {
		return 0, 0, false
	}

	return w, h, true
}
