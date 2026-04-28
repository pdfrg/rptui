// Package api handles external service integrations
package api

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pdfrg/rptui/internal/config"
	"github.com/pdfrg/rptui/internal/loginit"
	"github.com/pdfrg/rptui/internal/models"
)

var notifyLogger *log.Logger

func init() {
	notifyLogger = loginit.InitLogger("[NOTIFY] ")
}

var notifyArtPath = filepath.Join(os.TempDir(), "rptui-notify.jpg")

// SendDesktopNotification shows a desktop notification with song info
func SendDesktopNotification(song *models.Song, stationName string, cfg *config.Config, withImage bool) {
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

	switch runtime.GOOS {
	case "linux":
		sendLinuxNotification(title, body.String(), withImage, cfg)
	case "darwin":
		sendMacOSNotification(title, body.String(), withImage, cfg)
	case "windows":
		sendWindowsNotification(title, body.String(), withImage, cfg)
	}
}

func sendLinuxNotification(title, body string, withImage bool, cfg *config.Config) {
	if _, err := exec.LookPath("notify-send"); err != nil {
		return
	}

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

	args = append(args, "--", title, body)

	go func() {
		cmd := exec.Command("notify-send", args...)
		stderr, _ := cmd.StderrPipe()
		_ = cmd.Start()
		err := cmd.Wait()
		if err != nil {
			errBytes, _ := io.ReadAll(stderr)
			notifyLogger.Printf("notify-send args: %q", args)
			notifyLogger.Printf("desktop notification failed: %v, stderr: %s", err, string(errBytes))
		}
	}()
}

func sendMacOSNotification(title, body string, withImage bool, cfg *config.Config) {
	imgArg := ""
	if withImage {
		if _, err := os.Stat(notifyArtPath); err == nil {
			imgArg = fmt.Sprintf("with image alias POSIX file \"%s\"", notifyArtPath)
		}
	}

	script := fmt.Sprintf(`display notification "%s" with title "%s" %s`, strings.ReplaceAll(body, "\"", "\\\""), strings.ReplaceAll(title, "\"", "\\\""), imgArg)
	go func() {
		cmd := exec.Command("osascript", "-e", script)
		cmd.Stderr = os.Stderr
		_ = cmd.Start()
		_ = cmd.Wait()
	}()
}

func sendWindowsNotification(title, body string, withImage bool, cfg *config.Config) {
	// Windows PowerShell toast notification with optional image support
	// Replace newlines with </text><text> for multi-line in toast
	bodyEscaped := strings.ReplaceAll(body, "\n", "</text><text>")

	// Build XML with or without image
	var xmlContent string
	var usedImagePath string
	if withImage {
		// Try to find an image to use (same priority as Linux)
		var imagePath string
		if cfg.CopyAlbumArt && cfg.AlbumArtPath != "" {
			if _, err := os.Stat(cfg.AlbumArtPath); err == nil {
				imagePath = cfg.AlbumArtPath
			}
		} else if _, err := os.Stat(notifyArtPath); err == nil {
			imagePath = notifyArtPath
		}

		if imagePath != "" {
			// Use Windows-style paths for PowerShell (escape backslashes)
			usedImagePath = imagePath
			psPath := strings.ReplaceAll(imagePath, "\\", "\\\\")
			xmlContent = fmt.Sprintf("<toast><visual><binding template=\"ToastGeneric\"><image placement=\"appLogoOverride\" src=\"file:///%s\"/><text>%s</text><text>%s</text></binding></visual></toast>",
				psPath, strings.ReplaceAll(title, "&", "&amp;"), strings.ReplaceAll(bodyEscaped, "&", "&amp;"))
		} else {
			// Fallback to text-only if no image available
			xmlContent = fmt.Sprintf("<toast><visual><binding template=\"ToastGeneric\"><text>%s</text><text>%s</text></binding></visual></toast>",
				strings.ReplaceAll(title, "&", "&amp;"), strings.ReplaceAll(bodyEscaped, "&", "&amp;"))
		}
	} else {
		// Text-only notification
		xmlContent = fmt.Sprintf("<toast><visual><binding template=\"ToastGeneric\"><text>%s</text><text>%s</text></binding></visual></toast>",
			strings.ReplaceAll(title, "&", "&amp;"), strings.ReplaceAll(bodyEscaped, "&", "&amp;"))
	}

	script := fmt.Sprintf(`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml('%s')
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("rptui").Show($toast)`,
		xmlContent)

	go func() {
		cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
		if notifyLogger != nil {
			notifyLogger.Printf("Windows notification: title='%s', withImage=%v, imagePath='%s'", title, withImage, usedImagePath)
		}
		stderr, _ := cmd.StderrPipe()
		_ = cmd.Start()
		err := cmd.Wait()
		if err != nil {
			errBytes, _ := io.ReadAll(stderr)
			if notifyLogger != nil {
				notifyLogger.Printf("Windows notification failed: %v, stderr: %s", err, string(errBytes))
			}
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
