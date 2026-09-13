// Package config handles loading and saving user configuration.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// Window holds persisted window geometry.
type Window struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Maximized bool `json:"maximized"`
}

// Config is the application configuration stored as JSON.
type Config struct {
	Window         Window `json:"window"`
	Tray           bool   `json:"tray"`
	Notifications  bool   `json:"notifications"`
	StartMinimized bool   `json:"start_minimized"`

	mu   sync.Mutex
	path string
}

// Default returns the configuration used when no file exists yet.
func Default() *Config {
	return &Config{
		Window:        Window{Width: 1280, Height: 820, X: -1, Y: -1},
		Tray:          true,
		Notifications: true,
	}
}

// Load reads the configuration from path, falling back to defaults when the
// file does not exist.
func Load(path string) (*Config, error) {
	cfg := Default()
	cfg.path = path

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Save writes the configuration back to disk atomically.
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.path == "" {
		return errors.New("config: no path set")
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// SetWindow updates the stored window geometry.
func (c *Config) SetWindow(w Window) {
	c.mu.Lock()
	c.Window = w
	c.mu.Unlock()
}

// SetNotifications toggles desktop notifications.
func (c *Config) SetNotifications(enabled bool) {
	c.mu.Lock()
	c.Notifications = enabled
	c.mu.Unlock()
}

// NotificationsEnabled reports whether notifications are enabled.
func (c *Config) NotificationsEnabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Notifications
}
