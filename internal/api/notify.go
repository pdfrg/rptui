// Package api handles external service integrations
package api

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"rptui-bubbletea/internal/config"
	"rptui-bubbletea/internal/loginit"
	"rptui-bubbletea/internal/models"
)

var notifyLogger *log.Logger

func init() {
	notifyLogger = loginit.InitLogger("[NOTIFY] ")
}

const notifyArtPath = "/tmp/rptui-notify.jpg"

// SendDesktopNotification shows a desktop notification with song info
func SendDesktopNotification(song *models.Song, stationName string, cfg *config.Config, withImage bool) {
	if _, err := exec.LookPath("notify-send"); err != nil {
		return
	}

	title := "rptui - Radio Paradise"

	var body strings.Builder
	body.WriteString(song.Title)
	body.WriteString("\n")
	body.WriteString(song.Artist)
	body.WriteString("\n")

	if song.Album != "" && song.Album != "—" {
		if song.Year != "" && song.Year != "—" {
			body.WriteString(fmt.Sprintf("%s (%s)", song.Album, song.Year))
		} else {
			body.WriteString(song.Album)
		}
		body.WriteString("\n")
	}

	body.WriteString(stationName)

	var args []string
	args = append(args, "-t", "5000")

	if withImage {
		if cfg.CopyAlbumArt && cfg.AlbumArtPath != "" {
			if _, err := os.Stat(cfg.AlbumArtPath); err == nil {
				args = append(args, "-i", cfg.AlbumArtPath)
			}
		} else if _, err := os.Stat(notifyArtPath); err == nil {
			args = append(args, "-i", notifyArtPath)
		}
	}

	args = append(args, "--", title, body.String())

	go func() {
		cmd := exec.Command("notify-send", args...)
		stderr, _ := cmd.StderrPipe()
		cmd.Start()
		err := cmd.Wait()
		if err != nil {
			errBytes, _ := io.ReadAll(stderr)
			notifyLogger.Printf("notify-send args: %q", args)
			notifyLogger.Printf("desktop notification failed: %v, stderr: %s", err, string(errBytes))
		}
	}()
}

// SaveNotifyArt saves album art data to the notification art path
func SaveNotifyArt(imageData []byte) {
	if err := os.MkdirAll(filepath.Dir(notifyArtPath), 0755); err != nil {
		return
	}
	_ = os.WriteFile(notifyArtPath, imageData, 0644)
}
