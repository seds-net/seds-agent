package singbox

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Manager manages the sing-box subprocess
type Manager struct {
	mu          sync.RWMutex
	cmd         *exec.Cmd
	configPath  string
	execPath    string
	running     bool
	startTime   time.Time
	lastRestart time.Time
}

// Status represents sing-box status
type Status struct {
	Running   bool   `json:"running"`
	Version   string `json:"version"`
	Uptime    int64  `json:"uptime"`
	StartTime int64  `json:"start_time"`
}

// NewManager creates a new sing-box manager
func NewManager(execPath, configDir string) *Manager {
	return &Manager{
		execPath:   execPath,
		configPath: filepath.Join(configDir, "config.json"),
	}
}

// UpdateConfig writes new configuration to file
func (m *Manager) UpdateConfig(config json.RawMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure config directory exists
	if err := os.MkdirAll(filepath.Dir(m.configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config to file
	if err := os.WriteFile(m.configPath, config, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	log.Printf("Configuration updated at %s", m.configPath)
	return nil
}

// Start starts the sing-box process
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return fmt.Errorf("sing-box is already running")
	}

	// Check if config file exists
	if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", m.configPath)
	}

	// Create command
	m.cmd = exec.Command(m.execPath, "run", "-c", m.configPath)
	m.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Redirect output to logs
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr

	// Start process
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start sing-box: %w", err)
	}

	m.running = true
	m.startTime = time.Now()
	m.lastRestart = time.Now()

	log.Printf("sing-box started with PID %d", m.cmd.Process.Pid)

	// Monitor process in background
	go m.monitor()

	return nil
}

// Stop stops the sing-box process
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running || m.cmd == nil || m.cmd.Process == nil {
		return fmt.Errorf("sing-box is not running")
	}

	// Send SIGTERM
	if err := m.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// If SIGTERM fails, try SIGKILL
		if err := m.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill sing-box: %w", err)
		}
	}

	// Wait for process to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		done <- m.cmd.Wait()
	}()

	select {
	case <-done:
		m.running = false
		log.Println("sing-box stopped")
		return nil
	case <-time.After(10 * time.Second):
		// Force kill if not stopped after 10 seconds
		if err := m.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("failed to force kill sing-box: %w", err)
		}
		m.running = false
		log.Println("sing-box force killed")
		return nil
	}
}

// Restart restarts the sing-box process
func (m *Manager) Restart() error {
	if err := m.Stop(); err != nil {
		log.Printf("Warning: error stopping sing-box: %v", err)
	}

	// Wait a bit before restarting
	time.Sleep(1 * time.Second)

	return m.Start()
}

// GetStatus returns the current status
func (m *Manager) GetStatus() *Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := &Status{
		Running: m.running,
	}

	if m.running {
		status.Uptime = int64(time.Since(m.startTime).Seconds())
		status.StartTime = m.startTime.Unix()
	}

	return status
}

// IsRunning returns whether sing-box is running
func (m *Manager) IsRunning() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.running
}

// monitor monitors the process and marks it as stopped if it exits
func (m *Manager) monitor() {
	if m.cmd == nil {
		return
	}

	err := m.cmd.Wait()

	m.mu.Lock()
	m.running = false
	m.mu.Unlock()

	if err != nil {
		log.Printf("sing-box exited with error: %v", err)
	} else {
		log.Println("sing-box exited normally")
	}
}
