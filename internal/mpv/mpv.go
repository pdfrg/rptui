// Package mpv provides MPV media player backend with IPC control
package mpv

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Logger for MPV
var logger *log.Logger

func init() {
	f, err := os.OpenFile("rptui-go.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		logger = log.New(f, "[MPV] ", log.LstdFlags|log.Lshortfile)
	} else {
		logger = log.New(os.Stderr, "[MPV] ", log.LstdFlags|log.Lshortfile)
	}
}

// MPVBackend controls MPV via subprocess for audio playback
type MPVBackend struct {
	mu                   sync.Mutex
	process              *exec.Cmd
	currentURLs          []string
	isPaused             bool
	pauseStartTime       time.Time
	lastPlaybackPosition PlaybackPosition
	socketPath           string
	socketTimeout        time.Duration
	monitoringStop       chan struct{}
	monitoringDone       chan struct{}
}

// PlaybackPosition holds time and percent position
type PlaybackPosition struct {
	TimePos    float64 // Seconds
	PercentPos float64 // Percentage 0-100
}

// IPCCommand represents an MPV IPC command
type IPCCommand struct {
	Command []any `json:"command"`
}

// IPCResponse represents an MPV IPC response
type IPCResponse struct {
	Error string `json:"error"`
	Data  any    `json:"data"`
}

// NewMPVBackend creates a new MPV backend
func NewMPVBackend() *MPVBackend {
	// Use XDG runtime directory for socket
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		// Fallback to /tmp if XDG runtime not available
		runtimeDir = "/tmp"
	}
	mpvDir := filepath.Join(runtimeDir, "mpv")
	if err := os.MkdirAll(mpvDir, 0700); err != nil {
		// Fallback to /tmp if directory creation fails
		mpvDir = "/tmp"
	}

	socketPath := filepath.Join(mpvDir, "rptui-socket")

	return &MPVBackend{
		socketPath:    socketPath,
		socketTimeout: 2 * time.Second,
	}
}

// Start starts MPV with the given URLs
func (m *MPVBackend) Start(urls []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop any existing process
	m.stopLocked()

	// Ensure socket directory exists
	socketDir := filepath.Dir(m.socketPath)
	if err := os.MkdirAll(socketDir, 0700); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Remove stale socket file
	os.Remove(m.socketPath)

	// Build MPV command
	args := []string{
		"--no-video",
		"--force-window=no",
		"--no-terminal",
		"--gapless-audio=weak",
		fmt.Sprintf("--input-ipc-server=%s", m.socketPath),
	}
	args = append(args, urls...)

	logger.Printf("MPV Start: socket=%s, urls=%d", m.socketPath, len(urls))
	for i, url := range urls {
		logger.Printf("MPV URL[%d]: %s", i, url)
	}

	m.process = exec.Command("mpv", args...)
	m.process.Stdout = nil
	// Capture stderr to log any MPV errors
	stderrPipe, err := m.process.StderrPipe()
	if err != nil {
		logger.Printf("Failed to get stderr pipe: %v", err)
	}

	if err := m.process.Start(); err != nil {
		logger.Printf("MPV Start FAILED: %v", err)
		return fmt.Errorf("failed to start MPV: %w", err)
	}

	logger.Printf("MPV started with PID %d", m.process.Process.Pid)

	// Read stderr in background
	if stderrPipe != nil {
		go func() {
			buf := make([]byte, 4096)
			for {
				n, err := stderrPipe.Read(buf)
				if n > 0 {
					logger.Printf("MPV stderr: %s", string(buf[:n]))
				}
				if err != nil {
					break
				}
			}
		}()
	}

	m.currentURLs = urls
	m.isPaused = false
	m.pauseStartTime = time.Time{}

	// Wait a moment for socket to be created
	time.Sleep(200 * time.Millisecond)

	// Check if socket was created
	if _, err := os.Stat(m.socketPath); os.IsNotExist(err) {
		logger.Printf("WARNING: MPV socket not created at %s", m.socketPath)
	} else {
		logger.Printf("MPV socket exists at %s", m.socketPath)
	}

	return nil
}

// Stop stops playback
func (m *MPVBackend) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked()
}

// stopLocked stops playback (must be called with lock held)
func (m *MPVBackend) stopLocked() error {
	if m.process != nil {
		m.process.Process.Kill()
		m.process.Wait()
		m.process = nil
	}
	m.currentURLs = nil
	m.isPaused = false
	m.pauseStartTime = time.Time{}
	os.Remove(m.socketPath)
	return nil
}

// IsRunning checks if MPV process is running
func (m *MPVBackend) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil || m.process.Process == nil {
		return false
	}

	// Check if process is still running
	return m.process.ProcessState == nil || !m.process.ProcessState.Exited()
}

// IsPaused checks if MPV is paused
func (m *MPVBackend) IsPaused() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isPaused
}

// IsPlaying checks if MPV is running and not paused
func (m *MPVBackend) IsPlaying() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil || m.process.Process == nil {
		return false
	}
	if m.process.ProcessState != nil && m.process.ProcessState.Exited() {
		return false
	}
	return !m.isPaused
}

// TogglePause toggles pause/play state
func (m *MPVBackend) TogglePause() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil {
		return fmt.Errorf("MPV not running")
	}

	newPauseState := !m.isPaused

	cmd := IPCCommand{
		Command: []any{"set_property", "pause", newPauseState},
	}

	response, err := m.sendIPCCommandLocked(cmd)
	if err != nil {
		// Try to reconnect
		if m.reconnectLocked() {
			response, err = m.sendIPCCommandLocked(cmd)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	if response != nil {
		m.isPaused = newPauseState
		if m.isPaused {
			m.pauseStartTime = time.Now()
		} else {
			m.pauseStartTime = time.Time{}
		}
	}

	return nil
}

// Pause sets pause state
func (m *MPVBackend) Pause(pause bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil {
		return fmt.Errorf("MPV not running")
	}

	cmd := IPCCommand{
		Command: []any{"set_property", "pause", pause},
	}

	response, err := m.sendIPCCommandLocked(cmd)
	if err != nil {
		return err
	}

	if response != nil {
		m.isPaused = pause
		if m.isPaused {
			m.pauseStartTime = time.Now()
		} else {
			m.pauseStartTime = time.Time{}
		}
	}

	return nil
}

// SkipNext skips to next track in playlist
func (m *MPVBackend) SkipNext() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil {
		return fmt.Errorf("MPV not running")
	}

	cmd := IPCCommand{
		Command: []any{"playlist-next"},
	}

	_, err := m.sendIPCCommandLocked(cmd)
	return err
}

// SkipPrev skips to previous track in playlist
func (m *MPVBackend) SkipPrev() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil {
		return fmt.Errorf("MPV not running")
	}

	cmd := IPCCommand{
		Command: []any{"playlist-prev"},
	}

	_, err := m.sendIPCCommandLocked(cmd)
	return err
}

// SeekToStart seeks to beginning of current track
func (m *MPVBackend) SeekToStart() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil {
		return fmt.Errorf("MPV not running")
	}

	cmd := IPCCommand{
		Command: []any{"seek", 0, "absolute"},
	}

	_, err := m.sendIPCCommandLocked(cmd)
	return err
}

// GetPlaybackPosition gets current playback position
func (m *MPVBackend) GetPlaybackPosition() (PlaybackPosition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil {
		return PlaybackPosition{}, fmt.Errorf("MPV not running")
	}

	if m.isPaused {
		return m.lastPlaybackPosition, nil
	}

	// Get time-pos
	timeCmd := IPCCommand{
		Command: []any{"get_property", "time-pos"},
	}
	timeResp, err := m.sendIPCCommandLocked(timeCmd)
	if err != nil {
		return PlaybackPosition{}, err
	}

	timePos := 0.0
	if timeResp != nil && timeResp.Data != nil {
		if t, ok := timeResp.Data.(float64); ok {
			timePos = t
		}
	}

	// Get percent-pos
	percentCmd := IPCCommand{
		Command: []any{"get_property", "percent-pos"},
	}
	percentResp, err := m.sendIPCCommandLocked(percentCmd)
	if err != nil {
		return PlaybackPosition{}, err
	}

	percentPos := 0.0
	if percentResp != nil && percentResp.Data != nil {
		if p, ok := percentResp.Data.(float64); ok {
			percentPos = p
		}
	}

	pos := PlaybackPosition{
		TimePos:    timePos,
		PercentPos: percentPos,
	}
	m.lastPlaybackPosition = pos

	return pos, nil
}

// GetPlaylistPosition gets current playlist position (0-based)
func (m *MPVBackend) GetPlaylistPosition() (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil {
		return -1, fmt.Errorf("MPV not running")
	}

	cmd := IPCCommand{
		Command: []any{"get_property", "playlist-pos"},
	}

	resp, err := m.sendIPCCommandLocked(cmd)
	if err != nil {
		return -1, err
	}

	if resp == nil || resp.Data == nil {
		return -1, nil
	}

	if pos, ok := resp.Data.(float64); ok {
		return int(pos), nil
	}

	return -1, nil
}

// AppendToPlaylist appends URLs to current playlist
func (m *MPVBackend) AppendToPlaylist(urls []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.process == nil {
		return fmt.Errorf("MPV not running")
	}

	for _, url := range urls {
		cmd := IPCCommand{
			Command: []any{"loadfile", url, "append"},
		}
		_, err := m.sendIPCCommandLocked(cmd)
		if err != nil {
			return err
		}
	}

	return nil
}

// sendIPCCommandLocked sends an IPC command to MPV (must be called with lock held)
func (m *MPVBackend) sendIPCCommandLocked(cmd IPCCommand) (*IPCResponse, error) {
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{
		Name: m.socketPath,
		Net:  "unix",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MPV socket: %w", err)
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(m.socketTimeout))
	conn.SetWriteDeadline(time.Now().Add(m.socketTimeout))

	// Send command
	cmdData, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal command: %w", err)
	}
	cmdData = append(cmdData, '\n')

	if _, err := conn.Write(cmdData); err != nil {
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var response IPCResponse
	if err := json.Unmarshal(line, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if response.Error != "" && response.Error != "success" {
		return &response, fmt.Errorf("MPV error: %s", response.Error)
	}

	return &response, nil
}

// reconnectLocked tries to reconnect to MPV socket
func (m *MPVBackend) reconnectLocked() bool {
	// Test connection with simple command
	cmd := IPCCommand{
		Command: []any{"get_property", "pause"},
	}

	for i := 0; i < 3; i++ {
		response, err := m.sendIPCCommandLocked(cmd)
		if err == nil && response != nil && response.Error == "success" {
			return true
		}
		time.Sleep(1 * time.Second)
	}

	return false
}

// Restart restarts MPV with current playlist
func (m *MPVBackend) Restart() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentURLs == nil {
		return fmt.Errorf("no URLs to restart with")
	}

	// Copy URLs
	urls := make([]string, len(m.currentURLs))
	copy(urls, m.currentURLs)

	// Stop current process
	m.stopLocked()

	// Start with same URLs
	return m.Start(urls)
}

// GetCurrentURLs returns the current playlist URLs
func (m *MPVBackend) GetCurrentURLs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.currentURLs == nil {
		return nil
	}

	urls := make([]string, len(m.currentURLs))
	copy(urls, m.currentURLs)
	return urls
}

// GetSocketPath returns the socket path
func (m *MPVBackend) GetSocketPath() string {
	return m.socketPath
}
