//go:build linux

// Package app wires the window, tray, notifications and bridge together.
package app

import (
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"github.com/zealish/zealish-whatsapp/internal/bridge"
	"github.com/zealish/zealish-whatsapp/internal/config"
	"github.com/zealish/zealish-whatsapp/internal/notify"
	"github.com/zealish/zealish-whatsapp/internal/storage"
	"github.com/zealish/zealish-whatsapp/internal/tray"
	"github.com/zealish/zealish-whatsapp/internal/webview"
)

const (
	whatsappURL      = "https://web.whatsapp.com/"
	windowTitle      = "WhatsApp"
	trayIcon         = "com.zealish.WhatsApp"
	menuNotification = "Notifications"
)

// App is the running application instance.
type App struct {
	cfg   *config.Config
	paths storage.Paths
	log   *slog.Logger

	view     *webview.View
	tray     *tray.Tray
	notifier *notify.Notifier

	mu       sync.Mutex
	quitting bool
	// notifTags maps a notification tag back to the WebKit notification id so
	// clicks can be routed to the originating page notification.
	notifTags map[string]uint64
}

// New builds the application from configuration.
func New(cfg *config.Config, paths storage.Paths, log *slog.Logger) (*App, error) {
	a := &App{cfg: cfg, paths: paths, log: log, notifTags: make(map[string]uint64)}

	view, err := webview.New(webview.Options{
		Title:      windowTitle,
		URL:        whatsappURL,
		DataDir:    paths.Data,
		CacheDir:   paths.Cache,
		UserScript: bridge.UserScript(),
		Width:      cfg.Window.Width,
		Height:     cfg.Window.Height,
		Maximized:  cfg.Window.Maximized,
		Hidden:     cfg.StartMinimized && cfg.Tray,
		Handlers: webview.Handlers{
			Message:            a.onMessage,
			Notification:       a.onNotification,
			NotificationClosed: a.onNotificationClosed,
			ExternalURI:        a.openExternal,
			CloseRequest:       a.onCloseRequest,
			GeometryChanged:    a.onGeometryChanged,
		},
	})
	if err != nil {
		return nil, err
	}
	a.view = view

	if cfg.Notifications {
		a.setupNotifier()
	}
	if cfg.Tray {
		a.setupTray()
	}

	return a, nil
}

// Run enters the GTK main loop and returns the process exit code. It exits
// cleanly on SIGINT/SIGTERM so window geometry is always persisted.
func (a *App) Run() int {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)

	go func() {
		if _, ok := <-signals; ok {
			webview.Dispatch(a.Quit)
		}
	}()

	code := a.view.Run()
	a.shutdown()
	return code
}

func (a *App) setupNotifier() {
	notifier, err := notify.New(a.onNotificationActivated, nil)
	if err != nil {
		a.log.Warn("notifications unavailable", "error", err)
		return
	}
	a.notifier = notifier
}

func (a *App) setupTray() {
	enabled := a.cfg.NotificationsEnabled()
	items := []tray.Item{
		{Label: "Open", OnClick: func() { webview.Dispatch(a.view.Show) }},
		{Label: "Reload", OnClick: func() { webview.Dispatch(a.view.Reload) }},
		{Label: menuNotification, Checked: &enabled, OnClick: a.toggleNotifications},
		{Label: ""},
		{Label: "Quit", OnClick: func() { webview.Dispatch(a.Quit) }},
	}

	t, err := tray.New(trayIcon, windowTitle, items, func() {
		webview.Dispatch(a.toggleWindow)
	})
	if err != nil {
		a.log.Warn("tray unavailable", "error", err)
		return
	}
	a.tray = t
}

// Quit closes the window and terminates the main loop.
func (a *App) Quit() {
	a.mu.Lock()
	a.quitting = true
	a.mu.Unlock()
	a.view.Quit()
}

func (a *App) toggleWindow() {
	if a.view.Visible() {
		a.view.Hide()
	} else {
		a.view.Show()
	}
}

func (a *App) toggleNotifications() {
	enabled := !a.cfg.NotificationsEnabled()
	a.cfg.SetNotifications(enabled)
	if a.tray != nil {
		a.tray.SetChecked(menuNotification, enabled)
	}
	if err := a.cfg.Save(); err != nil {
		a.log.Warn("saving config failed", "error", err)
	}
}

func (a *App) onCloseRequest() bool {
	a.mu.Lock()
	quitting := a.quitting
	a.mu.Unlock()

	if quitting || a.tray == nil {
		return false
	}

	a.view.Hide()
	return true
}

func (a *App) onGeometryChanged(width, height int, maximized bool) {
	w := a.cfg.Window
	w.Width, w.Height, w.Maximized = width, height, maximized
	a.cfg.SetWindow(w)
}

func (a *App) onMessage(payload string) {
	msg, err := bridge.Decode(payload)
	if err != nil {
		a.log.Warn("ignoring bridge message", "error", err)
		return
	}

	switch msg.Type {
	case bridge.KindReady:
		a.log.Info("whatsapp ready")
	case bridge.KindUnread:
		a.log.Debug("unread changed", "count", msg.Unread)
	case bridge.KindOpenExternal:
		a.openExternal(msg.URL)
	}
}

func (a *App) onNotification(id uint64, title, body string) bool {
	// Notifications turned off: swallow the request entirely.
	if !a.cfg.NotificationsEnabled() {
		return true
	}
	// No DBus notifier: let WebKit fall back to its own presentation.
	if a.notifier == nil {
		return false
	}

	tag := title
	a.mu.Lock()
	a.notifTags[tag] = id
	a.mu.Unlock()

	err := a.notifier.Send(notify.Notification{
		Title: title,
		Body:  body,
		Icon:  trayIcon,
		Tag:   tag,
	})
	if err != nil {
		a.log.Warn("sending notification failed", "error", err)
		return false
	}
	return true
}

func (a *App) onNotificationClosed(id uint64) {
	a.mu.Lock()
	for tag, known := range a.notifTags {
		if known == id {
			delete(a.notifTags, tag)
		}
	}
	a.mu.Unlock()
}

func (a *App) onNotificationActivated(tag string) {
	a.mu.Lock()
	id, ok := a.notifTags[tag]
	a.mu.Unlock()

	webview.Dispatch(func() {
		a.view.Show()
		if ok {
			a.view.NotificationClicked(id)
		}
		a.view.Eval(bridge.FocusChat(tag))
	})
}

func (a *App) openExternal(uri string) {
	go func() {
		if err := exec.Command("xdg-open", uri).Start(); err != nil {
			a.log.Warn("opening external link failed", "error", err)
		}
	}()
}

func (a *App) shutdown() {
	width, height, maximized := a.view.Geometry()
	a.onGeometryChanged(width, height, maximized)
	if err := a.cfg.Save(); err != nil {
		a.log.Warn("saving config failed", "error", err)
	}

	if a.tray != nil {
		if err := a.tray.Close(); err != nil {
			a.log.Warn("closing tray failed", "error", err)
		}
	}
	if a.notifier != nil {
		if err := a.notifier.Close(); err != nil {
			a.log.Warn("closing notifier failed", "error", err)
		}
	}
}
