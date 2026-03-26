// Package loginit truncates the log file on startup and provides
// a helper to create prefixed loggers for each package.
// Import this package FIRST in main.go so truncation happens
// before other packages open the log file for appending.
package loginit

import (
	"log"
	"os"
)

const LogFile = "rptui.log"

func init() {
	// Truncate log file on each app start
	f, err := os.OpenFile(LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err == nil {
		f.Close()
	}
}

// InitLogger creates a logger with the given prefix that writes to the shared log file.
func InitLogger(prefix string) *log.Logger {
	f, err := os.OpenFile(LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return log.New(os.Stderr, prefix, log.LstdFlags|log.Lshortfile)
	}
	return log.New(f, prefix, log.LstdFlags|log.Lshortfile)
}
