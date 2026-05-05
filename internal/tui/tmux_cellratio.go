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
// This function uses three strategies in order of reliability:
// 1. tmux client_cell_width/height format variables — the simplest and most
// reliable approach. These are per-client values from the tmux server
// (available since tmux 3.0a, April 2019) that report the outer terminal's
// actual per-cell pixel size. No TTY manipulation or CSI queries needed.
// 2. CSI 16t (character cell size in pixels) — passes through tmux via
// DCS passthrough and returns the outer terminal's per-cell pixel size.
// 3. tmux client dimensions + CSI 14t — divides the outer terminal's total
// pixel dimensions (CSI 14t) by its character count (tmux client_width/height).
//
// Returns (fontWidth, fontHeight, true) if a correction was applied,
// or (0, 0, false) if no correction is needed or available.
func correctCellRatioForTmux(logger *log.Logger) (int, int, bool) {
	if !inTmux() {
		return 0, 0, false
	}

	if w, h, ok := getTmuxClientCellSize(); ok && w > 0 && h > 0 {
		logger.Printf("tmux cellRatio fix: client_cell returned %dx%d", w, h)
		return w, h, true
	}
	logger.Printf("tmux cellRatio fix: client_cell failed, trying CSI 16t")

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

func getTmuxClientCellSize() (width, height int, ok bool) {
	out, err := exec.Command("tmux", "display", "-p",
		"#{client_cell_width} #{client_cell_height}").Output()
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

func getTmuxPaneOffset() (rowOffset, colOffset int, ok bool) {
	if !inTmux() {
		return 0, 0, false
	}

	out, err := exec.Command("tmux", "display", "-p",
		"#{pane_top} #{pane_left} #{status-position}").Output()
	if err != nil {
		return 0, 0, false
	}

	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 3 {
		return 0, 0, false
	}

	paneTop, err := strconv.Atoi(fields[0])
	if err != nil || paneTop < 0 {
		return 0, 0, false
	}

	paneLeft, err := strconv.Atoi(fields[1])
	if err != nil || paneLeft < 0 {
		return 0, 0, false
	}

	statusTop := 0
	if fields[2] == "top" {
		statusTop = 1
	}

	return paneTop + statusTop, paneLeft, true
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

// ensureTmuxPassthroughAll sets tmux's allow-passthrough option to "all"
// at both pane and window level. go-termimg's enableTmuxPassthrough() sets
// it to "on" (value 1) at pane level, but value 1 still causes tmux to drop
// DCS passthrough when PANE_REDRAW or CLIENT_REDRAWPANES is set — which is
// almost always the case in multi-pane layouts. Value "all" (2) bypasses
// these checks, allowing passthrough even for invisible/redrawing panes.
//
// Both pane (-p) and window (-w) levels are set because pane options override
// window options in tmux's options_get lookup chain. go-termimg's sync.Once
// sets -p on, so we must override at -p level. We also set -w all so that
// other panes in the same window also benefit.
func ensureTmuxPassthroughAll(logger *log.Logger) {
	if !inTmux() {
		return
	}

	for _, args := range [][]string{
		{"set", "-p", "allow-passthrough", "all"},
		{"set", "-w", "allow-passthrough", "all"},
	} {
		if err := exec.Command("tmux", args...).Run(); err != nil {
			logger.Printf("Warning: tmux %v failed: %v", args, err)
		} else {
			logger.Printf("Set tmux allow-passthrough=all (%s)", args[1])
		}
	}
}

// detectTmuxOuterKitty checks whether the outer terminal (outside tmux)
// supports the Kitty graphics protocol by reading tmux's global environment
// variables. Inside tmux, os.Getenv("TERM_PROGRAM") returns "tmux" and
// os.Getenv("TERM") returns "tmux-256color", so go-termimg's normal
// environment-based detection cannot identify the outer terminal.
//
// This function queries `tmux show-environment -g` for TERM_PROGRAM and TERM,
// then checks if either contains any known Kitty-capable terminal name:
// rio, ghostty, wezterm, or kitty.
func detectTmuxOuterKitty() bool {
	if !inTmux() {
		return false
	}

	kittyNames := []string{"rio", "ghostty", "wezterm", "kitty"}

	match := func(val string) bool {
		lower := strings.ToLower(val)
		for _, name := range kittyNames {
			if strings.Contains(lower, name) {
				return true
			}
		}
		return false
	}

	for _, envVar := range []string{"TERM_PROGRAM", "TERM"} {
		out, err := exec.Command("tmux", "show-environment", "-g", envVar).Output()
		if err != nil {
			continue
		}
		line := strings.TrimSpace(string(out))
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && match(parts[1]) {
			return true
		}
	}

	return false
}

// detectTermKitty checks whether the current terminal supports the Kitty
// graphics protocol by examining the TERM environment variable. This
// supplements go-termimg's detection for the non-tmux SSH case where
// TERM_PROGRAM is not set (SSH only propagates TERM by default).
//
// go-termimg checks TERM_PROGRAM for "rio", "WezTerm", and "ghostty",
// but when connecting via SSH from these terminals, TERM_PROGRAM is
// typically empty. However, TERM is set (e.g., TERM=rio) and propagated
// by SSH. This function checks TERM against the known Kitty-capable
// terminal names, catching cases where go-termimg's TERM_PROGRAM-based
// detection fails and the terminal query times out over SSH.
func detectTermKitty() bool {
	if inTmux() {
		return false
	}

	term := strings.ToLower(os.Getenv("TERM"))
	for _, name := range []string{"rio", "ghostty", "wezterm", "kitty"} {
		if term == name || strings.Contains(term, name) {
			return true
		}
	}
	return false
}
