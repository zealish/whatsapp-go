// Package storage resolves XDG base directories used by the application.
package storage

import (
	"os"
	"path/filepath"
)

const appDir = "zealish-wa"

// Paths holds every directory the application writes to.
type Paths struct {
	Config string // ~/.config/zealish-wa
	Data   string // ~/.local/share/zealish-wa
	Cache  string // ~/.cache/zealish-wa
}

// Resolve returns the XDG paths for the application, creating them on demand.
func Resolve() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}

	p := Paths{
		Config: filepath.Join(xdg("XDG_CONFIG_HOME", home, ".config"), appDir),
		Data:   filepath.Join(xdg("XDG_DATA_HOME", home, ".local", "share"), appDir),
		Cache:  filepath.Join(xdg("XDG_CACHE_HOME", home, ".cache"), appDir),
	}

	for _, dir := range []string{p.Config, p.Data, p.Cache} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return Paths{}, err
		}
	}
	return p, nil
}

// ConfigFile returns the path of the JSON configuration file.
func (p Paths) ConfigFile() string { return filepath.Join(p.Config, "config.json") }

// LogDir returns the directory holding rotating log files.
func (p Paths) LogDir() string { return filepath.Join(p.Cache, "logs") }

func xdg(env, home string, fallback ...string) string {
	if v := os.Getenv(env); filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(append([]string{home}, fallback...)...)
}
