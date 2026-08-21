// Package loginit appends to the log file across sessions, rotating it at a
// size limit, and provides a helper to create prefixed loggers for each
// package. Import this package FIRST in main.go so rotation and the session
// marker happen before other packages open the log file for appending.
package loginit

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/adrg/xdg"
)

var LogFile string

// maxLogBytes bounds the log file size. On startup a larger log is rotated
// to <LogFile>.old so evidence from prior sessions (e.g. crash logs) is
// preserved without disk usage growing without bound.
const maxLogBytes = 5 << 20 // 5 MiB

func init() {
	stateDir := filepath.Join(xdg.StateHome, "rptui")
	_ = os.MkdirAll(stateDir, 0755)
	LogFile = filepath.Join(stateDir, "rptui.log")

	if info, err := os.Stat(LogFile); err == nil && info.Size() > maxLogBytes {
		_ = os.Rename(LogFile, LogFile+".old")
	}

	f, err := os.OpenFile(LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		_, _ = fmt.Fprintf(f, "=== rptui session started %s ===\n", time.Now().Format("2006/01/02 15:04:05"))
		_ = f.Close()
	}
}

func InitLogger(prefix string) *log.Logger {
	f, err := os.OpenFile(LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return log.New(os.Stderr, prefix, log.LstdFlags|log.Lshortfile)
	}
	return log.New(f, prefix, log.LstdFlags|log.Lshortfile)
}

// Recover is used as a deferred call in background goroutines to convert a
// panic into a logged error instead of crashing the whole process.
//
//	defer loginit.Recover(logger, "mpv process reaper")
func Recover(l *log.Logger, context string) {
	if r := recover(); r != nil {
		msg := fmt.Sprintf("background panic in %s: %v\n%s", context, r, debug.Stack())
		if l != nil {
			l.Print(msg)
		} else {
			log.Print(msg)
		}
	}
}
