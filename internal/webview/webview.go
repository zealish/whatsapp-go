//go:build linux

// Package webview launches a dedicated Chromium app window. Chromium is used
// instead of WebKitGTK because the system WebKitGTK build does not expose the
// WebRTC APIs required by WhatsApp calling.
package webview

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Handlers receives events from the browser adapter.
type Handlers struct {
	Message            func(payload string)
	Notification       func(id uint64, title, body string) bool
	NotificationClosed func(id uint64)
	ExternalURI        func(uri string)
	CloseRequest       func() bool
	GeometryChanged    func(width, height int, maximized bool)
}

// Options configures the dedicated Chromium app window.
type Options struct {
	Title      string
	URL        string
	DataDir    string
	CacheDir   string
	UserScript string
	Width      int
	Height     int
	Maximized  bool
	Hidden     bool
	Handlers   Handlers
}

// View controls the dedicated Chromium app window.
type View struct {
	opts     Options
	cmd      *exec.Cmd
	mu       sync.Mutex
	visible  bool
	quitting bool
}

// New creates a Chromium-backed WhatsApp window.
func New(opts Options) (*View, error) {
	chrome, err := findChrome()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(opts.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("webview: create data directory: %w", err)
	}
	if err := os.MkdirAll(opts.CacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("webview: create cache directory: %w", err)
	}
	profile := filepath.Join(opts.DataDir, "chrome-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return nil, fmt.Errorf("webview: create Chrome profile: %w", err)
	}
	if err := installTitleExtension(profile); err != nil {
		return nil, err
	}

	args := []string{
		"--app=" + opts.URL,
		"--user-data-dir=" + profile,
		"--disk-cache-dir=" + opts.CacheDir,
		"--class=WhatsApp",
		"--ozone-platform=x11",
		"--load-extension=" + filepath.Join(profile, "title-extension"),
		"--disable-extensions-except=" + filepath.Join(profile, "title-extension"),
		"--app=" + opts.URL,
		"--disable-session-crashed-bubble",
		"--window-size=" + strconv.Itoa(opts.Width) + "," + strconv.Itoa(opts.Height),
	}
	if opts.Maximized {
		args = append(args, "--start-maximized")
	}
	if opts.Hidden {
		args = append(args, "--start-minimized")
	}

	return &View{opts: opts, cmd: exec.Command(chrome, args...), visible: !opts.Hidden}, nil
}

func findChrome() (string, error) {
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("webview: Chromium/Google Chrome is required for WhatsApp calling")
}

func installTitleExtension(profile string) error {
	dir := filepath.Join(profile, "title-extension")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("webview: create title extension: %w", err)
	}
	manifest := `{"manifest_version":3,"name":"WhatsApp title","version":"1.0","content_scripts":[{"matches":["https://web.whatsapp.com/*"],"js":["title.js"],"run_at":"document_start","world":"MAIN"}]}`
	script := `(function(){var title="WhatsApp";function force(){try{var t=document.querySelector("title");if(t)t.textContent=title;document.title=title}catch(e){}}force();new MutationObserver(force).observe(document.documentElement||document,{subtree:true,childList:true,characterData:true});setInterval(force,100)})();`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o600); err != nil {
		return fmt.Errorf("webview: write title manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "title.js"), []byte(script), 0o600); err != nil {
		return fmt.Errorf("webview: write title script: %w", err)
	}
	return nil
}

// Run starts Chromium and waits for it to exit.
func (v *View) Run() int {
	if err := v.cmd.Start(); err != nil {
		return 1
	}
	go v.setWindowTitle()
	err := v.cmd.Wait()
	v.mu.Lock()
	v.visible = false
	v.mu.Unlock()
	if err != nil && !v.isQuitting() {
		return 1
	}
	return 0
}

// Quit terminates the Chromium process.
func (v *View) Quit() {
	v.mu.Lock()
	v.quitting = true
	cmd := v.cmd
	v.mu.Unlock()
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

// Show activates the Chromium window.
func (v *View) Show() {
	v.mu.Lock()
	v.visible = true
	v.mu.Unlock()
	_ = v.windowCommand("windowactivate")
}

// Hide minimizes the Chromium window.
func (v *View) Hide() {
	v.mu.Lock()
	v.visible = false
	v.mu.Unlock()
	_ = v.windowCommand("windowminimize")
}

// Visible reports the adapter's last known visibility state.
func (v *View) Visible() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.visible
}

// Reload reloads the WhatsApp page.
func (v *View) Reload() {
	_ = v.windowCommand("key", "ctrl+r")
}

// NotificationClicked is retained for API compatibility.
func (v *View) NotificationClicked(_ uint64) {}

// Eval is intentionally unavailable for an external Chrome app window. The
// WhatsApp page remains fully functional; native bridge actions are not used
// by the Chrome backend.
func (v *View) Eval(_ string) {}

// Geometry returns the configured window geometry.
func (v *View) Geometry() (width, height int, maximized bool) {
	return v.opts.Width, v.opts.Height, v.opts.Maximized
}

// Dispatch runs fn on the caller's thread.
func Dispatch(fn func()) { fn() }

func (v *View) isQuitting() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.quitting
}

func (v *View) setWindowTitle() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		if v.isQuitting() || v.cmd.ProcessState != nil {
			return
		}
		_ = v.windowCommand("set_window", "--name", "WhatsApp")
	}
}

func (v *View) windowCommand(args ...string) error {
	windowID, err := exec.Command("xdotool", "search", "--class", "WhatsApp").Output()
	if err != nil {
		return err
	}
	id := strings.Fields(string(windowID))
	if len(id) == 0 {
		return errors.New("webview: Chrome window not found")
	}
	var command []string
	switch args[0] {
	case "key":
		command = []string{"xdotool", "key", "--window", id[0], args[1]}
	case "set_window":
		command = []string{"xdotool", "set_window", "--name", args[2], id[0]}
	default:
		command = append([]string{"xdotool", args[0], id[0]}, args[1:]...)
	}
	return exec.Command(command[0], command[1:]...).Run()
}
