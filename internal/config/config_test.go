package config

import (
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Window.Width != 1280 || cfg.Window.Height != 820 {
		t.Errorf("got %dx%d, want 1280x820", cfg.Window.Width, cfg.Window.Height)
	}
	if !cfg.Tray || !cfg.Notifications {
		t.Error("tray and notifications should default to enabled")
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cfg.SetWindow(Window{Width: 1000, Height: 700, Maximized: true})
	cfg.SetNotifications(false)
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Window.Width != 1000 || got.Window.Height != 700 || !got.Window.Maximized {
		t.Errorf("window not persisted: %+v", got.Window)
	}
	if got.NotificationsEnabled() {
		t.Error("notifications should stay disabled")
	}
}
